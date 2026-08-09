package enrichment_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/ai"
	"tantan.local/tantan-api/internal/enrichment"
	"tantan.local/tantan-api/internal/jobs"
	"tantan.local/tantan-api/internal/storage"
	"tantan.local/tantan-api/internal/topic"
)

type fakeSecrets struct {
	values map[string]string
}

func (secrets *fakeSecrets) Get(_ context.Context, account string) (string, error) {
	value, ok := secrets.values[account]
	if !ok {
		return "", ai.ErrSecretNotFound
	}
	return value, nil
}

func (secrets *fakeSecrets) Set(_ context.Context, account, value string) error {
	secrets.values[account] = value
	return nil
}

func (secrets *fakeSecrets) Delete(_ context.Context, account string) error {
	delete(secrets.values, account)
	return nil
}

type generatorStep struct {
	output []byte
	err    error
}

type permanentProviderError struct{}

func (permanentProviderError) Error() string   { return "provider rejected request" }
func (permanentProviderError) Temporary() bool { return false }

type scriptedGenerator struct {
	mutex    sync.Mutex
	steps    []generatorStep
	requests []ai.GenerationRequest
}

type blockingGenerator struct {
	mutex   sync.Mutex
	active  int
	maximum int
	started chan struct{}
	release chan struct{}
}

type generatorFunc func(context.Context, string, ai.GenerationRequest) ([]byte, error)

func (function generatorFunc) Generate(ctx context.Context, apiKey string, request ai.GenerationRequest) ([]byte, error) {
	return function(ctx, apiKey, request)
}

func (generator *blockingGenerator) Generate(ctx context.Context, _ string, _ ai.GenerationRequest) ([]byte, error) {
	generator.mutex.Lock()
	generator.active++
	if generator.active > generator.maximum {
		generator.maximum = generator.active
	}
	generator.mutex.Unlock()
	generator.started <- struct{}{}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-generator.release:
	}
	generator.mutex.Lock()
	generator.active--
	generator.mutex.Unlock()
	return validEnrichmentOutput(), nil
}

func (generator *scriptedGenerator) Generate(_ context.Context, apiKey string, request ai.GenerationRequest) ([]byte, error) {
	if apiKey == "" {
		return nil, errors.New("missing API key")
	}
	generator.mutex.Lock()
	defer generator.mutex.Unlock()
	generator.requests = append(generator.requests, request)
	if len(generator.steps) == 0 {
		return nil, errors.New("unexpected generation")
	}
	step := generator.steps[0]
	generator.steps = generator.steps[1:]
	return step.output, step.err
}

func openEnrichmentFixture(t *testing.T) (*storage.Store, *ai.SettingsService, *topic.Service, time.Time, *fakeSecrets) {
	t.Helper()
	ctx := context.Background()
	store, err := storage.Open(ctx, storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	timestamp := now.Format(time.RFC3339Nano)
	hash := strings.Repeat("a", 64)
	if err := store.Write(ctx, func(transaction *sql.Tx) error {
		statements := []struct {
			query string
			args  []any
		}{
			{query: "INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES(?,?,?,?,?)", args: []any{"user_1", "AI User", "Asia/Shanghai", timestamp, timestamp}},
			{query: "INSERT INTO feeds(feed_id,title,view,updated_at) VALUES(?,?,?,?)", args: []any{"feed_1", "AI Source", 0, timestamp}},
			{query: "INSERT INTO entries(entry_id,feed_id,kind,title,content,media_json,published_at,content_hash,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)", args: []any{"entry_1", "feed_1", "article", "Agent workflow", "Original content", "[]", timestamp, hash, timestamp, timestamp}},
			{query: "INSERT INTO account_entries(user_id,entry_id,last_seen_at) VALUES(?,?,?)", args: []any{"user_1", "entry_1", timestamp}},
		}
		for _, statement := range statements {
			if _, err := transaction.ExecContext(ctx, statement.query, statement.args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("insert fixture: %v", err)
	}
	secrets := &fakeSecrets{values: map[string]string{ai.FixedProviderID: "fixture-enrichment-server-key"}}
	settings, err := ai.NewSettingsService(ai.SettingsConfig{Secrets: secrets})
	if err != nil {
		t.Fatal(err)
	}
	topics := topic.NewService(store, func() time.Time { return now })
	if err := topics.EnsureCore(ctx, "user_1"); err != nil {
		t.Fatalf("seed topics: %v", err)
	}
	return store, settings, topics, now, secrets
}

func validEnrichmentOutput() []byte {
	return []byte(`{"version":1,"detectedLanguage":"en","titleZh":"Agent 工作流","contentZh":"译文正文","summaryZh":"安全的 Agent 工作流摘要","keyPoints":["限制写入范围","验证结构化输出"]}`)
}

func validTopicOutput() []byte {
	return []byte(fmt.Sprintf(`{"version":1,"topics":[{"topicId":"%s","confidence":0.98,"reason":"AI"},{"topicId":"%s","confidence":0.92,"reason":"Agent"}]}`, topic.CoreID("user_1", "ai"), topic.CoreID("user_1", "agent")))
}

func TestWorkerRepairsJSONOnceThenAtomicallyCommitsEnrichmentTopicsAndFTS(t *testing.T) {
	ctx := context.Background()
	store, settings, topics, now, _ := openEnrichmentFixture(t)
	generator := &scriptedGenerator{steps: []generatorStep{{output: []byte(`{"invalid":true}`)}, {output: validEnrichmentOutput()}, {output: validTopicOutput()}}}
	service, err := enrichment.NewService(enrichment.Config{Store: store, Settings: settings, Generator: generator, Topics: topics, Now: func() time.Time { return now }, PromptVersion: "prompt-v1"})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := service.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: "entry_1", Language: "zh-CN", Fields: []string{"translation", "summary", "keyPoints", "topics"}})
	if err != nil || accepted.JobID == "" {
		t.Fatalf("ensure=%#v err=%v", accepted, err)
	}
	if ran, err := service.RunOne(ctx); err != nil || !ran {
		t.Fatalf("run worker=%t err=%v", ran, err)
	}
	result, err := service.Get(ctx, "user_1", "entry_1", "zh-CN")
	if err != nil || result.State != "ready" || result.Data == nil || result.Data.SummaryZh == "" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if len(generator.requests) != 3 || generator.requests[0].Repair || !generator.requests[1].Repair || generator.requests[2].SchemaName != ai.TopicSchemaName {
		t.Fatalf("generation requests=%#v", generator.requests)
	}
	var topicCount, searchCount int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM entry_topics WHERE user_id='user_1' AND entry_id='entry_1'").Scan(&topicCount); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM entry_search WHERE user_id='user_1' AND entry_id='entry_1' AND translation MATCH '译文正文'").Scan(&searchCount); err != nil {
		t.Fatal(err)
	}
	if topicCount != 2 || searchCount != 1 {
		t.Fatalf("topicCount=%d searchCount=%d", topicCount, searchCount)
	}
}

func TestInvalidSecondAIOutputNeverCommitsDerivedData(t *testing.T) {
	ctx := context.Background()
	store, settings, topics, now, _ := openEnrichmentFixture(t)
	generator := &scriptedGenerator{steps: []generatorStep{{output: []byte(`not-json`)}, {output: []byte(`{"still":"invalid"}`)}}}
	service, err := enrichment.NewService(enrichment.Config{Store: store, Settings: settings, Generator: generator, Topics: topics, Now: func() time.Time { return now }, PromptVersion: "prompt-v1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: "entry_1", Language: "zh-CN", Fields: []string{"translation", "summary"}}); err != nil {
		t.Fatal(err)
	}
	if ran, err := service.RunOne(ctx); err != nil || !ran {
		t.Fatalf("run=%t err=%v", ran, err)
	}
	result, err := service.Get(ctx, "user_1", "entry_1", "zh-CN")
	if err != nil || result.State != "failed" || result.ErrorCode != "AI_OUTPUT_INVALID" || result.Data != nil {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	var translated int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM entry_enrichments WHERE translated_title IS NOT NULL OR translated_content IS NOT NULL OR summary_text IS NOT NULL").Scan(&translated); err != nil || translated != 0 {
		t.Fatalf("derived rows=%d err=%v", translated, err)
	}
	if len(generator.requests) != 2 || !generator.requests[1].Repair {
		t.Fatalf("repair calls=%#v", generator.requests)
	}
}

func TestProviderFailureKeepsOriginalEntryAndQueuesBoundedRetry(t *testing.T) {
	ctx := context.Background()
	store, settings, topics, now, _ := openEnrichmentFixture(t)
	generator := &scriptedGenerator{steps: []generatorStep{{err: ai.ErrProviderUnavailable}}}
	service, err := enrichment.NewService(enrichment.Config{Store: store, Settings: settings, Generator: generator, Topics: topics, Now: func() time.Time { return now }, PromptVersion: "prompt-v1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: "entry_1", Language: "zh-CN", Fields: []string{"summary"}}); err != nil {
		t.Fatal(err)
	}
	if ran, err := service.RunOne(ctx); err != nil || !ran {
		t.Fatalf("run=%t err=%v", ran, err)
	}
	var content, state string
	var attempts int
	if err := store.DB().QueryRowContext(ctx, "SELECT content FROM entries WHERE entry_id='entry_1'").Scan(&content); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT state,attempts FROM jobs WHERE kind='enrich'").Scan(&state, &attempts); err != nil {
		t.Fatal(err)
	}
	if content != "Original content" || state != "queued" || attempts != 1 {
		t.Fatalf("content=%q state=%s attempts=%d", content, state, attempts)
	}
}

func TestPermanentProviderFailureIsTerminalWithoutRetry(t *testing.T) {
	ctx := context.Background()
	store, settings, topics, now, _ := openEnrichmentFixture(t)
	generator := &scriptedGenerator{steps: []generatorStep{{err: permanentProviderError{}}}}
	service, err := enrichment.NewService(enrichment.Config{Store: store, Settings: settings, Generator: generator, Topics: topics, Now: func() time.Time { return now }, PromptVersion: "prompt-v1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: "entry_1", Language: "zh-CN", Fields: []string{"summary"}}); err != nil {
		t.Fatal(err)
	}
	if ran, err := service.RunOne(ctx); err != nil || !ran {
		t.Fatalf("run=%t err=%v", ran, err)
	}
	var jobState, enrichmentState, errorCode string
	var attempts int
	if err := store.DB().QueryRowContext(ctx, "SELECT state,attempts,error_code FROM jobs WHERE kind='enrich'").Scan(&jobState, &attempts, &errorCode); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT state,error_code FROM entry_enrichments WHERE entry_id='entry_1'").Scan(&enrichmentState, &errorCode); err != nil {
		t.Fatal(err)
	}
	if jobState != "failed" || enrichmentState != "failed" || attempts != 1 || errorCode != "AI_PROVIDER_UNAVAILABLE" {
		t.Fatalf("job=%s enrichment=%s attempts=%d code=%s", jobState, enrichmentState, attempts, errorCode)
	}
}

func TestEnsureDeduplicatesByEntryFingerprintLanguageAndMergesFields(t *testing.T) {
	ctx := context.Background()
	store, settings, topics, now, _ := openEnrichmentFixture(t)
	service, err := enrichment.NewService(enrichment.Config{Store: store, Settings: settings, Generator: &scriptedGenerator{}, Topics: topics, Now: func() time.Time { return now }, PromptVersion: "prompt-v1"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: "entry_1", Language: "zh-CN", Fields: []string{"summary"}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: "entry_1", Language: "zh-CN", Fields: []string{"translation", "topics"}})
	if err != nil {
		t.Fatal(err)
	}
	if first.JobID != second.JobID {
		t.Fatalf("duplicate ensure created jobs %q and %q", first.JobID, second.JobID)
	}
	var count int
	var raw string
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*),MAX(payload_json) FROM jobs WHERE kind='enrich' AND state IN ('queued','running')").Scan(&count, &raw); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Fields []string `json:"fields"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	if count != 1 || strings.Join(payload.Fields, ",") != "summary,topics,translation" {
		t.Fatalf("count=%d fields=%v payload=%s", count, payload.Fields, raw)
	}
}

func TestEnsureDoesNotResetAnActiveEnrichmentToQueued(t *testing.T) {
	ctx := context.Background()
	store, settings, topics, now, _ := openEnrichmentFixture(t)
	service, err := enrichment.NewService(enrichment.Config{Store: store, Settings: settings, Generator: &scriptedGenerator{}, Topics: topics, Now: func() time.Time { return now }, PromptVersion: "prompt-v1"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: "entry_1", Language: "zh-CN", Fields: []string{"summary"}})
	if err != nil {
		t.Fatal(err)
	}
	claimed, found, err := jobs.ClaimNext(ctx, store, "enrich", now, time.Minute, 2)
	if err != nil || !found || claimed.ID != first.JobID {
		t.Fatalf("claim=%#v found=%t err=%v", claimed, found, err)
	}
	if _, err := store.DB().ExecContext(ctx, "UPDATE entry_enrichments SET state='processing' WHERE entry_id='entry_1'"); err != nil {
		t.Fatal(err)
	}
	second, err := service.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: "entry_1", Language: "zh-CN", Fields: []string{"topics"}})
	if err != nil || second.JobID != first.JobID {
		t.Fatalf("second=%#v err=%v", second, err)
	}
	var state string
	if err := store.DB().QueryRowContext(ctx, "SELECT state FROM entry_enrichments WHERE entry_id='entry_1'").Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "processing" {
		t.Fatalf("active enrichment reset to %q", state)
	}
}

func TestReadyEnrichmentWithoutTopicsQueuesLaterTopicRequest(t *testing.T) {
	ctx := context.Background()
	store, settings, topics, now, _ := openEnrichmentFixture(t)
	generator := &scriptedGenerator{steps: []generatorStep{{output: validEnrichmentOutput()}}}
	service, err := enrichment.NewService(enrichment.Config{Store: store, Settings: settings, Generator: generator, Topics: topics, Now: func() time.Time { return now }, PromptVersion: "prompt-v1"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: "entry_1", Language: "zh-CN", Fields: []string{"summary"}})
	if err != nil {
		t.Fatal(err)
	}
	if ran, err := service.RunOne(ctx); err != nil || !ran {
		t.Fatalf("run=%t err=%v", ran, err)
	}
	second, err := service.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: "entry_1", Language: "zh-CN", Fields: []string{"topics"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(second.JobID, "job_ready_") || second.JobID == first.JobID {
		t.Fatalf("later topic request was not queued: first=%s second=%s", first.JobID, second.JobID)
	}
}

func TestWorkerCannotOverwriteNewContentEnrichmentAfterProviderReturns(t *testing.T) {
	ctx := context.Background()
	store, settings, topics, now, _ := openEnrichmentFixture(t)
	var service *enrichment.Service
	mutated := false
	generator := generatorFunc(func(_ context.Context, _ string, _ ai.GenerationRequest) ([]byte, error) {
		if !mutated {
			mutated = true
			newHash := strings.Repeat("b", 64)
			if _, err := store.DB().ExecContext(ctx, "UPDATE entries SET content='new content',content_hash=? WHERE entry_id='entry_1'", newHash); err != nil {
				t.Fatalf("update entry during provider call: %v", err)
			}
			if _, err := service.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: "entry_1", Language: "zh-CN", Fields: []string{"summary"}}); err != nil {
				t.Fatalf("ensure new content during provider call: %v", err)
			}
		}
		return validEnrichmentOutput(), nil
	})
	var err error
	service, err = enrichment.NewService(enrichment.Config{Store: store, Settings: settings, Generator: generator, Topics: topics, Now: func() time.Time { return now }, PromptVersion: "prompt-v1"})
	if err != nil {
		t.Fatal(err)
	}
	old, err := service.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: "entry_1", Language: "zh-CN", Fields: []string{"summary"}})
	if err != nil {
		t.Fatal(err)
	}
	if ran, err := service.RunOne(ctx); err != nil || !ran {
		t.Fatalf("old worker run=%t err=%v", ran, err)
	}
	var hash, enrichmentState string
	if err := store.DB().QueryRowContext(ctx, "SELECT content_hash,state FROM entry_enrichments WHERE entry_id='entry_1'").Scan(&hash, &enrichmentState); err != nil {
		t.Fatal(err)
	}
	var oldState string
	if err := store.DB().QueryRowContext(ctx, "SELECT state FROM jobs WHERE job_id=?", old.JobID).Scan(&oldState); err != nil {
		t.Fatal(err)
	}
	var newJobs int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE kind='enrich' AND state='queued'").Scan(&newJobs); err != nil {
		t.Fatal(err)
	}
	if hash != strings.Repeat("b", 64) || enrichmentState != "queued" || oldState != "cancelled" || newJobs != 1 {
		t.Fatalf("hash=%s enrichment=%s oldJob=%s newJobs=%d", hash, enrichmentState, oldState, newJobs)
	}
}

func TestWorkerRevalidatesContentAndServerKeyBeforeCommit(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *storage.Store, *fakeSecrets)
	}{
		{
			name: "content hash",
			mutate: func(t *testing.T, store *storage.Store, _ *fakeSecrets) {
				t.Helper()
				if _, err := store.DB().ExecContext(context.Background(), "UPDATE entries SET content='changed during AI',content_hash=? WHERE entry_id='entry_1'", strings.Repeat("d", 64)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "server key removed",
			mutate: func(t *testing.T, _ *storage.Store, secrets *fakeSecrets) {
				t.Helper()
				delete(secrets.values, ai.FixedProviderID)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store, settings, topics, now, secrets := openEnrichmentFixture(t)
			mutated := false
			generator := generatorFunc(func(_ context.Context, _ string, _ ai.GenerationRequest) ([]byte, error) {
				if !mutated {
					mutated = true
					test.mutate(t, store, secrets)
				}
				return validEnrichmentOutput(), nil
			})
			service, err := enrichment.NewService(enrichment.Config{Store: store, Settings: settings, Generator: generator, Topics: topics, Now: func() time.Time { return now }, PromptVersion: "prompt-v1"})
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := service.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: "entry_1", Language: "zh-CN", Fields: []string{"summary"}})
			if err != nil {
				t.Fatal(err)
			}
			if ran, err := service.RunOne(ctx); err != nil || !ran {
				t.Fatalf("run=%t err=%v", ran, err)
			}
			var enrichmentState, jobState string
			if err := store.DB().QueryRowContext(ctx, "SELECT state FROM entry_enrichments WHERE entry_id='entry_1'").Scan(&enrichmentState); err != nil {
				t.Fatal(err)
			}
			if err := store.DB().QueryRowContext(ctx, "SELECT state FROM jobs WHERE job_id=?", accepted.JobID).Scan(&jobState); err != nil {
				t.Fatal(err)
			}
			if enrichmentState != "stale" || jobState != "cancelled" {
				t.Fatalf("enrichment=%s job=%s", enrichmentState, jobState)
			}
		})
	}
}

func TestGetNeverServesReadyDataForAChangedContentHash(t *testing.T) {
	ctx := context.Background()
	store, settings, topics, now, _ := openEnrichmentFixture(t)
	generator := &scriptedGenerator{steps: []generatorStep{{output: validEnrichmentOutput()}}}
	service, err := enrichment.NewService(enrichment.Config{Store: store, Settings: settings, Generator: generator, Topics: topics, Now: func() time.Time { return now }, PromptVersion: "prompt-v1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: "entry_1", Language: "zh-CN", Fields: []string{"summary"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RunOne(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx, "UPDATE entries SET content_hash=? WHERE entry_id='entry_1'", strings.Repeat("e", 64)); err != nil {
		t.Fatal(err)
	}
	result, err := service.Get(ctx, "user_1", "entry_1", "zh-CN")
	if err != nil || result.State != "missing" || result.Data != nil {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestPromptAndContentChangesCreateNewDedupeIdentityAndStaleOldRows(t *testing.T) {
	ctx := context.Background()
	store, settings, topics, now, _ := openEnrichmentFixture(t)
	service, err := enrichment.NewService(enrichment.Config{Store: store, Settings: settings, Generator: &scriptedGenerator{}, Topics: topics, Now: func() time.Time { return now }, PromptVersion: "prompt-v1"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: "entry_1", Language: "zh-CN", Fields: []string{"summary"}})
	if err != nil {
		t.Fatal(err)
	}
	serviceV2, err := enrichment.NewService(enrichment.Config{Store: store, Settings: settings, Generator: &scriptedGenerator{}, Topics: topics, Now: func() time.Time { return now.Add(time.Second) }, PromptVersion: "prompt-v2"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := serviceV2.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: "entry_1", Language: "zh-CN", Fields: []string{"summary"}})
	if err != nil || second.JobID == first.JobID {
		t.Fatalf("second=%#v err=%v", second, err)
	}
	newHash := strings.Repeat("b", 64)
	if _, err := store.DB().ExecContext(ctx, "UPDATE entries SET content='changed',content_hash=? WHERE entry_id='entry_1'", newHash); err != nil {
		t.Fatal(err)
	}
	third, err := serviceV2.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: "entry_1", Language: "zh-CN", Fields: []string{"summary"}})
	if err != nil || third.JobID == second.JobID {
		t.Fatalf("third=%#v err=%v", third, err)
	}
	var stale, queued int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FILTER (WHERE state='stale'),COUNT(*) FILTER (WHERE state='queued') FROM entry_enrichments WHERE entry_id='entry_1'").Scan(&stale, &queued); err != nil {
		t.Fatal(err)
	}
	if stale < 1 || queued != 1 {
		t.Fatalf("stale=%d queued=%d", stale, queued)
	}
	var activeJobs int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE kind='enrich' AND state IN ('queued','running')").Scan(&activeJobs); err != nil {
		t.Fatal(err)
	}
	if activeJobs != 3 {
		t.Fatalf("active enrichment jobs=%d", activeJobs)
	}
}

func TestEnrichmentProviderConcurrencyNeverExceedsTwo(t *testing.T) {
	ctx := context.Background()
	store, settings, topics, now, _ := openEnrichmentFixture(t)
	timestamp := now.Format(time.RFC3339Nano)
	hash := strings.Repeat("c", 64)
	if err := store.Write(ctx, func(transaction *sql.Tx) error {
		for index := 2; index <= 3; index++ {
			entryID := fmt.Sprintf("entry_%d", index)
			if _, err := transaction.ExecContext(ctx, "INSERT INTO entries(entry_id,feed_id,kind,title,content,media_json,published_at,content_hash,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)", entryID, "feed_1", "article", entryID, "content", "[]", timestamp, hash, timestamp, timestamp); err != nil {
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
	generator := &blockingGenerator{started: make(chan struct{}, 3), release: make(chan struct{}, 3)}
	service, err := enrichment.NewService(enrichment.Config{Store: store, Settings: settings, Generator: generator, Topics: topics, Now: func() time.Time { return now }, PromptVersion: "prompt-v1"})
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 3; index++ {
		if _, err := service.Ensure(ctx, enrichment.EnsureRequest{UserID: "user_1", EntryID: fmt.Sprintf("entry_%d", index), Language: "zh-CN", Fields: []string{"summary"}}); err != nil {
			t.Fatal(err)
		}
	}
	errorsChannel := make(chan error, 3)
	for range 3 {
		go func() {
			_, err := service.RunOne(ctx)
			errorsChannel <- err
		}()
	}
	for range 2 {
		select {
		case <-generator.started:
		case <-time.After(time.Second):
			t.Fatal("two provider calls did not start")
		}
	}
	select {
	case <-generator.started:
		t.Fatal("third provider call bypassed concurrency limit")
	case <-time.After(100 * time.Millisecond):
	}
	generator.release <- struct{}{}
	select {
	case <-generator.started:
	case <-time.After(time.Second):
		t.Fatal("third provider call did not start after capacity released")
	}
	generator.release <- struct{}{}
	generator.release <- struct{}{}
	for range 3 {
		if err := <-errorsChannel; err != nil {
			t.Fatalf("worker error: %v", err)
		}
	}
	if generator.maximum != 2 {
		t.Fatalf("maximum provider concurrency=%d", generator.maximum)
	}
}
