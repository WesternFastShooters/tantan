package search_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"tantan.local/tantan-api/internal/search"
	"tantan.local/tantan-api/internal/storage"
)

func openSearchStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(context.Background(), storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func insertSearchFixture(t *testing.T, store *storage.Store) {
	t.Helper()
	ctx := context.Background()
	indexer := search.NewIndexer(store)
	now := "2026-08-09T10:00:00Z"
	hash := strings.Repeat("a", 64)
	err := store.Write(ctx, func(tx *sql.Tx) error {
		statements := []struct {
			query string
			args  []any
		}{
			{query: "INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES(?,?,?,?,?)", args: []any{"user_1", "Test User", "Asia/Shanghai", now, now}},
			{query: "INSERT INTO feeds(feed_id,title,url,image,view,updated_at) VALUES(?,?,?,?,?,?)", args: []any{"feed_1", "SourceToken Weekly", "https://source.invalid/feed", nil, 0, now}},
			{query: "INSERT INTO entries(entry_id,feed_id,kind,title,description,content,author,url,language,media_json,published_at,content_hash,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)", args: []any{"entry_main", "feed_1", "article", "TitleToken report", "description", "BodyToken original CommonNeedle", "Author", "https://content.invalid/main", "en", "[]", now, hash, now, now}},
			{query: "INSERT INTO entries(entry_id,feed_id,kind,title,description,content,author,url,language,media_json,published_at,content_hash,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)", args: []any{"entry_read", "feed_1", "post", "Another item", "description", "CommonNeedle second item", "Author", "https://content.invalid/read", "en", "[]", "2026-08-09T09:59:00Z", hash, now, now}},
			{query: "INSERT INTO account_entries(user_id,entry_id,read_at,collected_at,last_seen_at) VALUES(?,?,?,?,?)", args: []any{"user_1", "entry_main", nil, nil, now}},
			{query: "INSERT INTO account_entries(user_id,entry_id,read_at,collected_at,last_seen_at) VALUES(?,?,?,?,?)", args: []any{"user_1", "entry_read", now, now, now}},
			{query: "INSERT INTO entry_enrichments(entry_id,provider_fp,language,state,translated_title,translated_content,summary_text,key_points_json,tags_json,quality_score,content_hash,prompt_version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)", args: []any{"entry_main", "provider0001", "zh-CN", "ready", "译文标题", "译文命中内容", "summary", "[]", `["TagToken"]`, 8, hash, "v1", now, now}},
			{query: "INSERT INTO topics(topic_id,user_id,name,normalized_name,kind,pinned,hidden,sort_order,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)", args: []any{"topic_1", "user_1", "TopicToken", "topictoken", "core", 0, 0, 10, now, now}},
			{query: "INSERT INTO entry_topics(user_id,entry_id,topic_id,confidence,is_primary,content_hash,created_at) VALUES(?,?,?,?,?,?,?)", args: []any{"user_1", "entry_main", "topic_1", 0.9, 1, hash, now}},
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
				return err
			}
		}
		return indexer.RefreshTx(ctx, tx, "user_1", []string{"entry_main", "entry_read"})
	})
	if err != nil {
		t.Fatalf("insert search fixture: %v", err)
	}
}

func newSearchService(t *testing.T, store *storage.Store) *search.Service {
	t.Helper()
	service, err := search.NewService(search.Config{
		Store:     store,
		CursorKey: []byte("fixed-search-cursor-key-for-tests-32"),
	})
	if err != nil {
		t.Fatalf("create search service: %v", err)
	}
	return service
}

func resultContains(items []search.Item, entryID string) bool {
	for _, item := range items {
		if item.EntryID == entryID {
			return true
		}
	}
	return false
}

func TestSearchCoversOriginalTranslationSourceTopicTagAndReadStates(t *testing.T) {
	ctx := context.Background()
	store := openSearchStore(t)
	insertSearchFixture(t, store)
	service := newSearchService(t, store)

	queries := []string{"TitleToken", "BodyToken", "译文命中", "SourceToken", "TopicToken", "TagToken"}
	for _, query := range queries {
		page, err := service.Search(ctx, search.Query{UserID: "user_1", Text: query, Limit: 20})
		if err != nil {
			t.Fatalf("search %q: %v", query, err)
		}
		if !resultContains(page.Items, "entry_main") {
			t.Fatalf("search %q items=%#v", query, page.Items)
		}
		if page.IndexStatus != "ready" {
			t.Fatalf("search %q indexStatus=%q", query, page.IndexStatus)
		}
	}

	page, err := service.Search(ctx, search.Query{UserID: "user_1", Text: "CommonNeedle", Limit: 20})
	if err != nil {
		t.Fatalf("search read states: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("read-state item count=%d", len(page.Items))
	}
	readStates := map[string]bool{}
	for _, item := range page.Items {
		readStates[item.EntryID] = item.Read
	}
	if readStates["entry_main"] || !readStates["entry_read"] {
		t.Fatalf("read states=%v", readStates)
	}
}

func TestSearchCursorIsSignedBoundToQueryAndDoesNotDuplicate(t *testing.T) {
	ctx := context.Background()
	store := openSearchStore(t)
	insertSearchFixture(t, store)
	service := newSearchService(t, store)
	first, err := service.Search(ctx, search.Query{UserID: "user_1", Text: "CommonNeedle", Limit: 1})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Items) != 1 || first.NextCursor == nil {
		t.Fatalf("first page=%#v", first)
	}
	second, err := service.Search(ctx, search.Query{UserID: "user_1", Text: "CommonNeedle", Limit: 1, Cursor: *first.NextCursor})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].EntryID == first.Items[0].EntryID || second.NextCursor != nil {
		t.Fatalf("second page=%#v", second)
	}

	tampered := *first.NextCursor
	if tampered[len(tampered)-1] == 'A' {
		tampered = tampered[:len(tampered)-1] + "B"
	} else {
		tampered = tampered[:len(tampered)-1] + "A"
	}
	if _, err := service.Search(ctx, search.Query{UserID: "user_1", Text: "CommonNeedle", Limit: 1, Cursor: tampered}); !errors.Is(err, search.ErrCursorInvalid) {
		t.Fatalf("tampered cursor error=%v", err)
	}
	if _, err := service.Search(ctx, search.Query{UserID: "user_1", Text: "DifferentQuery", Limit: 1, Cursor: *first.NextCursor}); !errors.Is(err, search.ErrCursorMismatch) {
		t.Fatalf("cross-query cursor error=%v", err)
	}
}

func TestSearchIndexStatusReflectsBuildingAndCorruption(t *testing.T) {
	ctx := context.Background()
	store := openSearchStore(t)
	insertSearchFixture(t, store)
	indexer := search.NewIndexer(store)
	if _, err := store.DB().ExecContext(ctx, "DELETE FROM entry_search WHERE entry_id='entry_main' AND user_id='user_1'"); err != nil {
		t.Fatalf("remove index row: %v", err)
	}
	if status, err := indexer.Status(ctx, "user_1"); err != nil || status != "building" {
		t.Fatalf("building status=%q err=%v", status, err)
	}
	if _, err := store.DB().ExecContext(ctx, `
INSERT INTO entry_search(entry_id,user_id,title,translation,content,source,topics,tags)
VALUES('orphan_entry','user_1','orphan','','','','','')`); err != nil {
		t.Fatalf("insert orphan index row: %v", err)
	}
	if status, err := indexer.Status(ctx, "user_1"); err != nil || status != "degraded" {
		t.Fatalf("degraded status=%q err=%v", status, err)
	}
}

func TestRefreshInvalidatesDerivedDataWhenContentHashChanges(t *testing.T) {
	ctx := context.Background()
	store := openSearchStore(t)
	insertSearchFixture(t, store)
	indexer := search.NewIndexer(store)
	newHash := strings.Repeat("b", 64)
	if err := store.Write(ctx, func(transaction *sql.Tx) error {
		if _, err := transaction.ExecContext(ctx, `
UPDATE entries SET content='FreshBodyToken',content_hash=?,updated_at='2026-08-09T11:00:00Z'
WHERE entry_id='entry_main'`, newHash); err != nil {
			return err
		}
		return indexer.RefreshTx(ctx, transaction, "user_1", []string{"entry_main"})
	}); err != nil {
		t.Fatal(err)
	}
	var enrichmentState string
	if err := store.DB().QueryRowContext(ctx, "SELECT state FROM entry_enrichments WHERE entry_id='entry_main'").Scan(&enrichmentState); err != nil {
		t.Fatal(err)
	}
	if enrichmentState != "stale" {
		t.Fatalf("changed-content enrichment state=%q", enrichmentState)
	}
	service := newSearchService(t, store)
	for _, oldDerivedTerm := range []string{"译文命中", "TopicToken", "TagToken"} {
		page, err := service.Search(ctx, search.Query{UserID: "user_1", Text: oldDerivedTerm, Limit: 20})
		if err != nil {
			t.Fatalf("search stale term %q: %v", oldDerivedTerm, err)
		}
		if resultContains(page.Items, "entry_main") {
			t.Fatalf("stale derived term %q remained searchable", oldDerivedTerm)
		}
	}
	page, err := service.Search(ctx, search.Query{UserID: "user_1", Text: "FreshBodyToken", Limit: 20})
	if err != nil || !resultContains(page.Items, "entry_main") {
		t.Fatalf("fresh original search=%#v err=%v", page, err)
	}
}
