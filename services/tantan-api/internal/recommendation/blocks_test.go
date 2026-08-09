package recommendation_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/recommendation"
	"tantan.local/tantan-api/internal/storage"
)

func TestSourceBlocksCanBeListedAndRestoredIdempotently(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	timestamp := now.Format(time.RFC3339Nano)
	if err := store.Write(ctx, func(transaction *sql.Tx) error {
		for _, statement := range []string{
			"INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES('user_blocks','Blocks User','Asia/Shanghai','" + timestamp + "','" + timestamp + "')",
			"INSERT INTO feeds(feed_id,title,view,updated_at) VALUES('source_blocked','Blocked Source',0,'" + timestamp + "')",
			"INSERT INTO entries(entry_id,feed_id,kind,title,media_json,published_at,content_hash,created_at,updated_at) VALUES('entry_blocked','source_blocked','article','Entry','[]','" + timestamp + "','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','" + timestamp + "','" + timestamp + "')",
			"INSERT INTO account_entries(user_id,entry_id,last_seen_at) VALUES('user_blocks','entry_blocked','" + timestamp + "')",
		} {
			if _, err := transaction.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	service, err := recommendation.NewFeedbackService(recommendation.FeedbackConfig{
		Store: store,
		Now:   func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Apply(ctx, recommendation.FeedbackRequest{
		UserID:         "user_blocks",
		EntryID:        "entry_blocked",
		Action:         "block_source",
		IdempotencyKey: "block-source-key-0001",
	}); err != nil {
		t.Fatal(err)
	}

	blocks, err := service.ListSourceBlocks(ctx, "user_blocks")
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || blocks[0].SourceID != "source_blocked" || blocks[0].Name != "Blocked Source" {
		t.Fatalf("blocks=%#v", blocks)
	}

	restore := recommendation.RestoreSourceBlockRequest{
		UserID:         "user_blocks",
		SourceID:       "source_blocked",
		IdempotencyKey: "restore-source-key-0001",
	}
	if err := service.RestoreSourceBlock(ctx, restore); err != nil {
		t.Fatal(err)
	}
	if err := service.RestoreSourceBlock(ctx, restore); err != nil {
		t.Fatalf("idempotent replay failed: %v", err)
	}
	blocks, err = service.ListSourceBlocks(ctx, "user_blocks")
	if err != nil || len(blocks) != 0 {
		t.Fatalf("blocks after restore=%#v err=%v", blocks, err)
	}
	var undoEvents int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM recommendation_events WHERE user_id='user_blocks' AND event_type='undo'").Scan(&undoEvents); err != nil {
		t.Fatal(err)
	}
	if undoEvents != 1 {
		t.Fatalf("undo events=%d, want 1", undoEvents)
	}
}
