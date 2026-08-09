package filter_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/ai"
	"tantan.local/tantan-api/internal/filter"
	"tantan.local/tantan-api/internal/home"
	"tantan.local/tantan-api/internal/storage"
	"tantan.local/tantan-api/internal/topic"
)

type filterSecrets struct{ values map[string]string }

func (secrets *filterSecrets) Get(_ context.Context, account string) (string, error) {
	value, ok := secrets.values[account]
	if !ok {
		return "", ai.ErrSecretNotFound
	}
	return value, nil
}
func (secrets *filterSecrets) Set(_ context.Context, account, value string) error {
	secrets.values[account] = value
	return nil
}
func (secrets *filterSecrets) Delete(_ context.Context, account string) error {
	delete(secrets.values, account)
	return nil
}

type filterStep struct {
	output []byte
	err    error
}

type filterGenerator struct {
	mutex    sync.Mutex
	steps    []filterStep
	requests []ai.GenerationRequest
}

func (generator *filterGenerator) Generate(_ context.Context, _ string, request ai.GenerationRequest) ([]byte, error) {
	generator.mutex.Lock()
	defer generator.mutex.Unlock()
	generator.requests = append(generator.requests, request)
	if len(generator.steps) == 0 {
		return nil, errors.New("unexpected filter generation")
	}
	step := generator.steps[0]
	generator.steps = generator.steps[1:]
	return step.output, step.err
}

func validFilterOutput() []byte {
	return []byte(`{"version":1,"windowDays":7,"includeTopics":[],"excludeTopics":[],"includeSources":[],"excludeSources":[],"includeTerms":["Title"],"negativeTerms":[],"languages":["en"],"contentStyles":["article","post","image","video"],"weights":{"freshness":1,"topicMatch":1,"sourceAffinity":1,"quality":1,"diversity":1}}`)
}

func openFilterFixture(t *testing.T) (*storage.Store, *ai.SettingsService, *home.Service, *topic.Service, time.Time) {
	t.Helper()
	ctx := context.Background()
	store, err := storage.Open(ctx, storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	timestamp := now.Format(time.RFC3339Nano)
	if err := store.Write(ctx, func(transaction *sql.Tx) error {
		if _, err := transaction.ExecContext(ctx, "INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES(?,?,?,?,?)", "user_1", "Filter User", "Asia/Shanghai", timestamp, timestamp); err != nil {
			return err
		}
		for source := 0; source < 6; source++ {
			if _, err := transaction.ExecContext(ctx, "INSERT INTO feeds(feed_id,title,view,updated_at) VALUES(?,?,0,?)", fmt.Sprintf("feed_%d", source), fmt.Sprintf("Source %d", source), timestamp); err != nil {
				return err
			}
		}
		for index := 0; index < 30; index++ {
			entryID := fmt.Sprintf("entry_%02d", index)
			hash := fmt.Sprintf("%064x", index+1)
			published := now.Add(-time.Duration(index) * time.Minute).Format(time.RFC3339Nano)
			if _, err := transaction.ExecContext(ctx, "INSERT INTO entries(entry_id,feed_id,kind,title,content,language,media_json,published_at,content_hash,created_at,updated_at) VALUES(?,?,?,?,?,'en','[]',?,?,?,?)", entryID, fmt.Sprintf("feed_%d", index%6), []string{"article", "post", "image", "video"}[index%4], "Title "+entryID, "Body", published, hash, timestamp, timestamp); err != nil {
				return err
			}
			if _, err := transaction.ExecContext(ctx, "INSERT INTO account_entries(user_id,entry_id,last_seen_at) VALUES(?,?,?)", "user_1", entryID, timestamp); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	settings, err := ai.NewSettingsService(ai.SettingsConfig{Secrets: &filterSecrets{values: map[string]string{ai.FixedProviderID: "fixture-filter-server-key"}}})
	if err != nil {
		t.Fatal(err)
	}
	homeService, err := home.NewService(home.Config{Store: store, CursorKey: []byte("filter-home-cursor-key-thirty-two!!"), Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	topics := topic.NewService(store, func() time.Time { return now })
	return store, settings, homeService, topics, now
}

func TestFilterRepairsOnceSwitchesAtomicallyAndResetRestoresDefault(t *testing.T) {
	ctx := context.Background()
	store, settings, homeService, topics, now := openFilterFixture(t)
	defaultPage, err := homeService.Get(ctx, home.Query{UserID: "user_1", TopicID: "recommend", Timezone: "Asia/Shanghai", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	generator := &filterGenerator{steps: []filterStep{{output: []byte(`{"invalid":true}`)}, {output: validFilterOutput()}, {output: []byte(`not-json`)}, {output: []byte(`{"still":"invalid"}`)}, {output: validFilterOutput()}}}
	service, err := filter.NewService(filter.Config{Store: store, Settings: settings, Generator: generator, Home: homeService, Topics: topics, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	request := filter.Request{UserID: "user_1", Prompt: "多推 Title 相关内容", Timezone: "Asia/Shanghai", IdempotencyKey: "filter-request-key-0001"}
	mutation, err := service.Put(ctx, request)
	if err != nil || mutation.Filter == nil || mutation.QueueID == defaultPage.Queue.ID || mutation.TopicsRevision < 1 || mutation.QueueGeneration == "" {
		t.Fatalf("mutation=%#v err=%v", mutation, err)
	}
	var storedGeneration string
	if err := store.DB().QueryRowContext(ctx, "SELECT generation FROM daily_queues WHERE queue_id=?", mutation.QueueID).Scan(&storedGeneration); err != nil || storedGeneration != mutation.QueueGeneration {
		t.Fatalf("stored generation=%q mutation=%#v err=%v", storedGeneration, mutation, err)
	}
	if len(generator.requests) != 2 || generator.requests[0].Repair || !generator.requests[1].Repair {
		t.Fatalf("requests=%#v", generator.requests)
	}
	replayed, err := service.Put(ctx, request)
	if err != nil || replayed.QueueID != mutation.QueueID || replayed.QueueGeneration != mutation.QueueGeneration || replayed.TopicsRevision != mutation.TopicsRevision || replayed.Filter == nil || replayed.Filter.ID != mutation.Filter.ID || len(generator.requests) != 2 {
		t.Fatalf("replayed=%#v requests=%d err=%v", replayed, len(generator.requests), err)
	}
	conflict := request
	conflict.Prompt = "同一个幂等键不能换 Prompt"
	if _, err := service.Put(ctx, conflict); !errors.Is(err, filter.ErrIdempotencyConflict) {
		t.Fatalf("filter idempotency conflict=%v", err)
	}
	var activeFilters, readyQueues, filterTopics, assignments int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM home_filters WHERE user_id='user_1' AND status='active'").Scan(&activeFilters); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM daily_queues WHERE queue_id=? AND status='ready'", mutation.QueueID).Scan(&readyQueues); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM topics WHERE user_id='user_1' AND kind='filter'").Scan(&filterTopics); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM entry_topics et JOIN topics t ON t.topic_id=et.topic_id WHERE t.kind='filter' AND t.user_id='user_1'").Scan(&assignments); err != nil {
		t.Fatal(err)
	}
	if activeFilters != 1 || readyQueues != 1 || filterTopics != 1 || assignments == 0 {
		t.Fatalf("active=%d queue=%d topics=%d assignments=%d", activeFilters, readyQueues, filterTopics, assignments)
	}
	firstFilterID := mutation.Filter.ID
	firstQueueID := mutation.QueueID
	firstTopicID := ""
	for _, item := range mutation.Topics {
		if item.Kind == "filter" {
			firstTopicID = item.ID
		}
	}
	if firstTopicID == "" {
		t.Fatal("filter topic missing from response")
	}
	if _, err := service.Put(ctx, filter.Request{UserID: "user_1", Prompt: "这次模型输出失败", Timezone: "Asia/Shanghai", IdempotencyKey: "filter-request-key-0002"}); !errors.Is(err, filter.ErrAIOutputInvalid) {
		t.Fatalf("invalid second filter error=%v", err)
	}
	var activeID string
	if err := store.DB().QueryRowContext(ctx, "SELECT filter_id FROM home_filters WHERE user_id='user_1' AND status='active'").Scan(&activeID); err != nil {
		t.Fatal(err)
	}
	var stillReady, stillTopic int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM daily_queues WHERE queue_id=? AND status='ready'", firstQueueID).Scan(&stillReady); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM topics WHERE topic_id=?", firstTopicID).Scan(&stillTopic); err != nil {
		t.Fatal(err)
	}
	if activeID != firstFilterID || stillReady != 1 || stillTopic != 1 {
		t.Fatalf("failed filter changed state: active=%s ready=%d topic=%d", activeID, stillReady, stillTopic)
	}
	if _, err := store.DB().ExecContext(ctx, `
CREATE TRIGGER fail_filter_topic BEFORE INSERT ON topics
WHEN NEW.kind='filter' BEGIN SELECT RAISE(ABORT,'injected filter transaction failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Put(ctx, filter.Request{UserID: "user_1", Prompt: "事务内失败也必须回滚", Timezone: "Asia/Shanghai", IdempotencyKey: "filter-request-key-0004"}); err == nil {
		t.Fatal("injected transaction failure was accepted")
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT filter_id FROM home_filters WHERE user_id='user_1' AND status='active'").Scan(&activeID); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM daily_queues WHERE queue_id=? AND status='ready'", firstQueueID).Scan(&stillReady); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM topics WHERE topic_id=?", firstTopicID).Scan(&stillTopic); err != nil {
		t.Fatal(err)
	}
	if activeID != firstFilterID || stillReady != 1 || stillTopic != 1 {
		t.Fatalf("rolled-back filter changed state: active=%s ready=%d topic=%d", activeID, stillReady, stillTopic)
	}
	reset, err := service.Delete(ctx, "user_1", "Asia/Shanghai")
	if err != nil || reset.Filter != nil || reset.QueueID != defaultPage.Queue.ID || reset.QueueGeneration != defaultPage.Queue.Generation || reset.TopicsRevision <= mutation.TopicsRevision {
		t.Fatalf("reset=%#v err=%v", reset, err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM home_filters WHERE user_id='user_1' AND status='active'").Scan(&activeFilters); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM topics WHERE user_id='user_1' AND kind='filter'").Scan(&filterTopics); err != nil {
		t.Fatal(err)
	}
	if activeFilters != 0 || filterTopics != 0 {
		t.Fatalf("reset active=%d filterTopics=%d", activeFilters, filterTopics)
	}
	if secondReset, err := service.Delete(ctx, "user_1", "Asia/Shanghai"); err != nil || secondReset.QueueID != defaultPage.Queue.ID {
		t.Fatalf("repeated reset=%#v err=%v", secondReset, err)
	}
}

func TestConcurrentFilterReplayGeneratesAndCommitsOnce(t *testing.T) {
	ctx := context.Background()
	store, settings, homeService, topics, now := openFilterFixture(t)
	generator := &filterGenerator{steps: []filterStep{{output: validFilterOutput()}}}
	service, err := filter.NewService(filter.Config{Store: store, Settings: settings, Generator: generator, Home: homeService, Topics: topics, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	request := filter.Request{UserID: "user_1", Prompt: "并发只生成一次", Timezone: "Asia/Shanghai", IdempotencyKey: "concurrent-filter-key-0001"}
	const workers = 8
	results := make(chan filter.Mutation, workers)
	errorsChannel := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			mutation, err := service.Put(ctx, request)
			if err != nil {
				errorsChannel <- err
				return
			}
			results <- mutation
		}()
	}
	group.Wait()
	close(results)
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatal(err)
	}
	queueID := ""
	filterID := ""
	for mutation := range results {
		if mutation.Filter == nil {
			t.Fatal("concurrent mutation omitted filter")
		}
		if queueID == "" {
			queueID = mutation.QueueID
			filterID = mutation.Filter.ID
		}
		if mutation.QueueID != queueID || mutation.Filter.ID != filterID {
			t.Fatalf("concurrent mutations diverged: queue=%s filter=%s", mutation.QueueID, mutation.Filter.ID)
		}
	}
	if len(generator.requests) != 1 {
		t.Fatalf("concurrent generation count=%d", len(generator.requests))
	}
	var filters, queues int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM home_filters WHERE status='active'").Scan(&filters); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM daily_queues WHERE status='ready' AND filter_key=?", filterID).Scan(&queues); err != nil {
		t.Fatal(err)
	}
	if filters != 1 || queues != 1 {
		t.Fatalf("filters=%d queues=%d", filters, queues)
	}
}

func TestProviderFailureLeavesExistingFilterUntouched(t *testing.T) {
	ctx := context.Background()
	store, settings, homeService, topics, now := openFilterFixture(t)
	if _, err := homeService.Get(ctx, home.Query{UserID: "user_1", TopicID: "recommend", Timezone: "Asia/Shanghai", Limit: 20}); err != nil {
		t.Fatal(err)
	}
	generator := &filterGenerator{steps: []filterStep{{err: ai.ErrProviderUnavailable}}}
	service, err := filter.NewService(filter.Config{Store: store, Settings: settings, Generator: generator, Home: homeService, Topics: topics, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Put(ctx, filter.Request{UserID: "user_1", Prompt: strings.Repeat("x", 10), Timezone: "Asia/Shanghai", IdempotencyKey: "filter-request-key-0003"}); !errors.Is(err, ai.ErrProviderUnavailable) {
		t.Fatalf("provider error=%v", err)
	}
	var filters int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM home_filters").Scan(&filters); err != nil || filters != 0 {
		t.Fatalf("filters=%d err=%v", filters, err)
	}
}
