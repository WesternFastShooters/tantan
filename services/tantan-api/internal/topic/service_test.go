package topic_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/ai"
	"tantan.local/tantan-api/internal/storage"
	"tantan.local/tantan-api/internal/topic"
)

func openTopicStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(context.Background(), storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func insertTopicFixture(t *testing.T, store *storage.Store) {
	t.Helper()
	ctx := context.Background()
	now := "2026-08-09T10:00:00Z"
	hash := strings.Repeat("a", 64)
	if err := store.Write(ctx, func(transaction *sql.Tx) error {
		statements := []struct {
			query string
			args  []any
		}{
			{query: "INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES(?,?,?,?,?)", args: []any{"user_1", "Topic User", "Asia/Shanghai", now, now}},
			{query: "INSERT INTO feeds(feed_id,title,view,updated_at) VALUES(?,?,?,?)", args: []any{"feed_1", "Topic Source", 0, now}},
			{query: "INSERT INTO entries(entry_id,feed_id,kind,title,content,media_json,published_at,content_hash,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)", args: []any{"entry_1", "feed_1", "article", "AI Agent", "content", "[]", now, hash, now, now}},
			{query: "INSERT INTO account_entries(user_id,entry_id,last_seen_at) VALUES(?,?,?)", args: []any{"user_1", "entry_1", now}},
		}
		for _, statement := range statements {
			if _, err := transaction.ExecContext(ctx, statement.query, statement.args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("insert topic fixture: %v", err)
	}
}

func TestCoreTopicsAreSeededIdempotentlyAndClassificationIsAtomic(t *testing.T) {
	ctx := context.Background()
	store := openTopicStore(t)
	insertTopicFixture(t, store)
	clock := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	service := topic.NewService(store, func() time.Time { return clock })
	for run := 0; run < 2; run++ {
		if err := service.EnsureCore(ctx, "user_1"); err != nil {
			t.Fatalf("seed core run %d: %v", run, err)
		}
	}
	list, err := service.List(ctx, "user_1")
	if err != nil {
		t.Fatalf("list topics: %v", err)
	}
	if list.Version < 1 || list.TopicsRevision < 1 || len(list.Topics) != 1 || list.Topics[0].ID != "recommend" || list.Topics[0].Kind != "virtual" {
		t.Fatalf("topics=%#v", list)
	}
	var internalCoreTopics int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM topics WHERE user_id='user_1' AND kind='core'").Scan(&internalCoreTopics); err != nil || internalCoreTopics != 6 {
		t.Fatalf("internal core topics=%d err=%v", internalCoreTopics, err)
	}
	aiTopicID := topic.CoreID("user_1", "ai")
	agentTopicID := topic.CoreID("user_1", "agent")
	classification := ai.TopicClassificationV1{Version: 1, Topics: []ai.TopicClassification{
		{TopicID: aiTopicID, Confidence: 0.98, Reason: "AI"},
		{TopicID: agentTopicID, Confidence: 0.92, Reason: "Agent"},
	}}
	if err := service.ApplyClassification(ctx, "user_1", "entry_1", strings.Repeat("a", 64), classification); err != nil {
		t.Fatalf("apply classification: %v", err)
	}
	var assignments, primary int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM(is_primary),0) FROM entry_topics WHERE user_id='user_1' AND entry_id='entry_1'").Scan(&assignments, &primary); err != nil {
		t.Fatal(err)
	}
	if assignments != 2 || primary != 1 {
		t.Fatalf("assignments=%d primary=%d", assignments, primary)
	}
	list, err = service.List(ctx, "user_1")
	if err != nil || len(list.Topics) != 1 {
		t.Fatalf("legacy core topics became visible: topics=%#v err=%v", list.Topics, err)
	}
}

func TestOnlyRecommendIsFixedAndGeneratedTopicsAreVisible(t *testing.T) {
	ctx := context.Background()
	store := openTopicStore(t)
	insertTopicFixture(t, store)
	service := topic.NewService(store, func() time.Time {
		return time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	})
	if err := service.EnsureCore(ctx, "user_1"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.EnsureDynamic(ctx, "user_1", "AI Agent"); err != nil {
		t.Fatal(err)
	}

	list, err := service.List(ctx, "user_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Topics) != 2 || list.Topics[0].ID != "recommend" || list.Topics[1].Name != "AI Agent" {
		t.Fatalf("visible topics=%#v", list.Topics)
	}
	if !list.Topics[0].Fixed || list.Topics[1].Fixed {
		t.Fatalf("fixed flags=%#v", list.Topics)
	}
}

func TestUnreadClassifierBuildsDeterministicTopicsAndAssignments(t *testing.T) {
	ctx := context.Background()
	store := openTopicStore(t)
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	timestamp := now.Format(time.RFC3339Nano)
	titles := []string{
		"Claude Agent workflow guide",
		"Claude Agent tool calling",
		"Claude Agent memory patterns",
		"SQLite text history prototype",
		"SQLite query planner notes",
		"SQLite database migration guide",
	}
	if err := store.Write(ctx, func(transaction *sql.Tx) error {
		if _, err := transaction.ExecContext(ctx, "INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES('user_classifier','Classifier','Asia/Shanghai',?,?)", timestamp, timestamp); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, "INSERT INTO feeds(feed_id,title,view,updated_at) VALUES('feed_classifier','Engineering',0,?)", timestamp); err != nil {
			return err
		}
		for index, title := range titles {
			entryID := "entry_classifier_" + string(rune('a'+index))
			hash := strings.Repeat(string(rune('a'+index)), 64)
			published := now.Add(-time.Duration(index) * time.Hour).Format(time.RFC3339Nano)
			if _, err := transaction.ExecContext(ctx, "INSERT INTO entries(entry_id,feed_id,kind,title,description,content,language,media_json,published_at,content_hash,created_at,updated_at) VALUES(?,'feed_classifier','article',?,?,'body','en','[]',?,?,?,?)", entryID, title, title, published, hash, timestamp, timestamp); err != nil {
				return err
			}
			if _, err := transaction.ExecContext(ctx, "INSERT INTO account_entries(user_id,entry_id,last_seen_at) VALUES('user_classifier',?,?)", entryID, timestamp); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service := topic.NewService(store, func() time.Time { return now })
	first, err := service.Classify(ctx, "user_classifier", nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Classify(ctx, "user_classifier", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Topics) < 2 || len(first.Topics) > 7 || !reflect.DeepEqual(first, second) {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
	visibleNames := make(map[string]bool, len(first.Topics))
	for _, item := range first.Topics {
		visibleNames[item.Name] = true
		for _, forbidden := range []string{"br", "RT", "target blank", "https"} {
			if item.Name == forbidden {
				t.Fatalf("markup/navigation token escaped classifier: %#v", first.Topics)
			}
		}
	}
	if !visibleNames["Agent"] || !visibleNames["编程"] {
		t.Fatalf("expected semantic topics, got %#v", first.Topics)
	}
	if err := service.ReplaceGenerated(ctx, "user_classifier", "dynamic", first); err != nil {
		t.Fatal(err)
	}
	list, err := service.List(ctx, "user_classifier")
	if err != nil || len(list.Topics) != len(first.Topics)+1 {
		t.Fatalf("visible=%#v generated=%#v err=%v", list.Topics, first.Topics, err)
	}
	for _, item := range list.Topics[1:] {
		if item.Fixed || item.UnreadCount < 2 {
			t.Fatalf("dynamic topic=%#v", item)
		}
	}
}

func TestTopicPatchUsesPersistentOptimisticVersionAndImmutableRecommend(t *testing.T) {
	ctx := context.Background()
	store := openTopicStore(t)
	insertTopicFixture(t, store)
	clock := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	service := topic.NewService(store, func() time.Time { return clock })
	if err := service.EnsureCore(ctx, "user_1"); err != nil {
		t.Fatal(err)
	}
	before, err := service.List(ctx, "user_1")
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Millisecond)
	aiTopicID := topic.CoreID("user_1", "ai")
	web3TopicID := topic.CoreID("user_1", "web3")
	after, err := service.Patch(ctx, "user_1", before.Version, []topic.Operation{{Op: "pin", TopicID: aiTopicID}, {Op: "hide", TopicID: web3TopicID}})
	if err != nil {
		t.Fatalf("patch topics: %v", err)
	}
	if after.Version <= before.Version {
		t.Fatalf("version before=%d after=%d", before.Version, after.Version)
	}
	if after.TopicsRevision <= before.TopicsRevision {
		t.Fatalf("topics revision before=%d after=%d", before.TopicsRevision, after.TopicsRevision)
	}
	if _, err := service.Patch(ctx, "user_1", before.Version, []topic.Operation{{Op: "show", TopicID: web3TopicID}}); !errors.Is(err, topic.ErrVersionConflict) {
		t.Fatalf("stale version error=%v", err)
	}
	if _, err := service.Patch(ctx, "user_1", after.Version, []topic.Operation{{Op: "hide", TopicID: "recommend"}}); err == nil {
		t.Fatal("virtual recommend topic was mutable")
	}
}

func TestNormalizeNameAppliesCompatibilityCaseAndWhitespaceRules(t *testing.T) {
	if got := topic.NormalizeName("  ＡＩ　 Coding\t"); got != "ai coding" {
		t.Fatalf("normalized=%q", got)
	}
	if got := topic.NormalizeName("\ufb00 ＣＯＤＥＸ \u212a"); got != "ff codex k" {
		t.Fatalf("compatibility normalized=%q", got)
	}
}

func TestDynamicTopicMergesNormalizedNamesAndStaysVisibleForSevenDays(t *testing.T) {
	ctx := context.Background()
	store := openTopicStore(t)
	insertTopicFixture(t, store)
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	service := topic.NewService(store, func() time.Time { return now })
	first, err := service.EnsureDynamic(ctx, "user_1", "  \uff23\uff2f\uff24\uff25\uff38  ")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.EnsureDynamic(ctx, "user_1", "codex")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.Name != "CODEX" || first.Kind != "dynamic" {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
	list, err := service.List(ctx, "user_1")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range list.Topics {
		if item.ID == first.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("new dynamic topic was hidden before stable_until")
	}
	var count int
	var stableUntil string
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*),MAX(stable_until) FROM topics WHERE user_id=? AND normalized_name='codex'", "user_1").Scan(&count, &stableUntil); err != nil {
		t.Fatal(err)
	}
	if count != 1 || stableUntil != now.Add(7*24*time.Hour).Format(time.RFC3339Nano) {
		t.Fatalf("count=%d stableUntil=%q", count, stableUntil)
	}
	if _, err := service.EnsureDynamic(ctx, "user_1", strings.Repeat("长", 21)); err == nil {
		t.Fatal("dynamic topic longer than 20 runes was accepted")
	}
}

func TestGeneratedTopicCanParticipateInCallerTransaction(t *testing.T) {
	ctx := context.Background()
	store := openTopicStore(t)
	insertTopicFixture(t, store)
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	service := topic.NewService(store, func() time.Time { return now })
	wantRollback := errors.New("rollback fixture")
	err := store.Write(ctx, func(transaction *sql.Tx) error {
		item, err := service.EnsureGeneratedTx(ctx, transaction, "user_1", "Claude Code", "filter")
		if err != nil || item.Kind != "filter" {
			t.Fatalf("item=%#v err=%v", item, err)
		}
		return wantRollback
	})
	if !errors.Is(err, wantRollback) {
		t.Fatalf("transaction error=%v", err)
	}
	var count int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM topics WHERE user_id='user_1'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("generated topic escaped rollback: count=%d", count)
	}
}

func TestTopicUnreadCountsIgnoreAssignmentsFromAnOldContentHash(t *testing.T) {
	ctx := context.Background()
	store := openTopicStore(t)
	insertTopicFixture(t, store)
	service := topic.NewService(store, func() time.Time { return time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC) })
	if err := service.EnsureCore(ctx, "user_1"); err != nil {
		t.Fatal(err)
	}
	aiTopicID := topic.CoreID("user_1", "ai")
	if err := service.ApplyClassification(ctx, "user_1", "entry_1", strings.Repeat("a", 64), ai.TopicClassificationV1{Version: 1, Topics: []ai.TopicClassification{{TopicID: aiTopicID, Confidence: 0.9, Reason: "AI"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx, "UPDATE entries SET content_hash=? WHERE entry_id='entry_1'", strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	list, err := service.List(ctx, "user_1")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list.Topics {
		if item.ID == aiTopicID && item.UnreadCount != 0 {
			t.Fatalf("old assignment unread count=%d", item.UnreadCount)
		}
	}
}
