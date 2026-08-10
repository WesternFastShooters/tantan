package contentpool_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/contentpool"
	"tantan.local/tantan-api/internal/storage"
)

func TestPoolReturnsChineseAndReadyTranslationsOnly(t *testing.T) {
	ctx := context.Background()
	store := openPoolStore(t, ctx)
	defer store.Close()
	seedPool(t, ctx, store)
	service, err := contentpool.NewService(contentpool.Config{Store: store, CursorKey: []byte(strings.Repeat("k", 32))})
	if err != nil {
		t.Fatal(err)
	}

	page, err := service.List(ctx, contentpool.Query{UserID: "user_pool", SourceID: "feed_pool", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Pool.Total != 3 || page.Pool.Ready != 2 || page.Pool.Pending != 1 {
		t.Fatalf("pool=%+v", page.Pool)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items=%+v", page.Items)
	}
	if page.Items[0].Title != "中文原文" || page.Items[0].Translated {
		t.Fatalf("Chinese item=%+v", page.Items[0])
	}
	if page.Items[1].Title != "英文内容的中文标题" || page.Items[1].Excerpt == nil || *page.Items[1].Excerpt != "英文内容的中文正文" || !page.Items[1].Translated {
		t.Fatalf("translated item=%+v", page.Items[1])
	}
	for _, item := range page.Items {
		if strings.Contains(item.Title, "English") || (item.Excerpt != nil && strings.Contains(*item.Excerpt, "English")) {
			t.Fatalf("untranslated text escaped: %+v", item)
		}
	}
}

func TestPoolCursorIsStableSignedAndSourceScoped(t *testing.T) {
	ctx := context.Background()
	store := openPoolStore(t, ctx)
	defer store.Close()
	seedPool(t, ctx, store)
	service, err := contentpool.NewService(contentpool.Config{Store: store, CursorKey: []byte(strings.Repeat("c", 32))})
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.List(ctx, contentpool.Query{UserID: "user_pool", SourceID: "feed_pool", Limit: 1})
	if err != nil || len(first.Items) != 1 || first.NextCursor == nil {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := service.List(ctx, contentpool.Query{UserID: "user_pool", SourceID: "feed_pool", Limit: 1, Cursor: *first.NextCursor})
	if err != nil || len(second.Items) != 1 || second.Items[0].EntryID == first.Items[0].EntryID {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	mutated := *first.NextCursor
	if mutated[len(mutated)-1] == 'A' {
		mutated = mutated[:len(mutated)-1] + "B"
	} else {
		mutated = mutated[:len(mutated)-1] + "A"
	}
	if _, err := service.List(ctx, contentpool.Query{UserID: "user_pool", SourceID: "feed_pool", Limit: 1, Cursor: mutated}); !strings.Contains(err.Error(), "cursor") {
		t.Fatalf("tampered cursor err=%v", err)
	}
	if _, err := service.List(ctx, contentpool.Query{UserID: "user_pool", SourceID: "feed_other", Limit: 1, Cursor: *first.NextCursor}); !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("cross-source cursor err=%v", err)
	}
}

func openPoolStore(t *testing.T, ctx context.Context) *storage.Store {
	t.Helper()
	store, err := storage.Open(ctx, storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func seedPool(t *testing.T, ctx context.Context, store *storage.Store) {
	t.Helper()
	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	timestamp := now.Format(time.RFC3339Nano)
	if err := store.Write(ctx, func(transaction *sql.Tx) error {
		statements := []string{
			"INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES('user_pool','Pool User','Asia/Shanghai','" + timestamp + "','" + timestamp + "')",
			"INSERT INTO feeds(feed_id,title,url,view,updated_at) VALUES('feed_pool','Pool RSS','https://pool.invalid/rss',0,'" + timestamp + "')",
			"INSERT INTO feeds(feed_id,title,url,view,updated_at) VALUES('feed_other','Other RSS','https://other.invalid/rss',0,'" + timestamp + "')",
			"INSERT INTO entries(entry_id,feed_id,kind,title,description,content,language,media_json,published_at,content_hash,created_at,updated_at) VALUES('entry_zh','feed_pool','article','中文原文','中文摘要','中文正文','zh-CN','[]','" + now.Format(time.RFC3339Nano) + "','" + strings.Repeat("a", 64) + "','" + timestamp + "','" + timestamp + "')",
			"INSERT INTO entries(entry_id,feed_id,kind,title,description,content,language,media_json,published_at,content_hash,created_at,updated_at) VALUES('entry_ready','feed_pool','article','English ready','English excerpt','English body','en','[]','" + now.Add(-time.Hour).Format(time.RFC3339Nano) + "','" + strings.Repeat("b", 64) + "','" + timestamp + "','" + timestamp + "')",
			"INSERT INTO entries(entry_id,feed_id,kind,title,description,content,language,media_json,published_at,content_hash,created_at,updated_at) VALUES('entry_pending','feed_pool','article','English pending','English pending excerpt','English pending body','en','[]','" + now.Add(-2*time.Hour).Format(time.RFC3339Nano) + "','" + strings.Repeat("c", 64) + "','" + timestamp + "','" + timestamp + "')",
			"INSERT INTO account_entries(user_id,entry_id,last_seen_at) VALUES('user_pool','entry_zh','" + timestamp + "')",
			"INSERT INTO account_entries(user_id,entry_id,last_seen_at) VALUES('user_pool','entry_ready','" + timestamp + "')",
			"INSERT INTO account_entries(user_id,entry_id,last_seen_at) VALUES('user_pool','entry_pending','" + timestamp + "')",
			"INSERT INTO entry_enrichments(entry_id,provider_fp,language,state,translated_title,translated_content,summary_text,key_points_json,tags_json,quality_score,content_hash,prompt_version,created_at,updated_at) VALUES('entry_ready','ABCDEF123456','zh-CN','ready','英文内容的中文标题','英文内容的中文正文','中文摘要','[\"要点\"]','[]',10,'" + strings.Repeat("b", 64) + "','tantan-ai-v1','" + timestamp + "','" + timestamp + "')",
		}
		for _, statement := range statements {
			if _, err := transaction.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
