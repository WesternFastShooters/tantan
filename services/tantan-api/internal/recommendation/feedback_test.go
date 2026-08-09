package recommendation_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/recommendation"
	"tantan.local/tantan-api/internal/storage"
)

func TestFeedbackIsIdempotentUpdatesBlocksAndNeverReaddsCurrentQueue(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	timestamp := now.Format(time.RFC3339Nano)
	hash := strings.Repeat("a", 64)
	if err := store.Write(ctx, func(transaction *sql.Tx) error {
		statements := []string{
			"INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES('user_1','Feedback User','Asia/Shanghai','" + timestamp + "','" + timestamp + "')",
			"INSERT INTO feeds(feed_id,title,view,updated_at) VALUES('feed_1','Source',0,'" + timestamp + "')",
			"INSERT INTO entries(entry_id,feed_id,kind,title,media_json,published_at,content_hash,created_at,updated_at) VALUES('entry_1','feed_1','article','Entry','[]','" + timestamp + "','" + hash + "','" + timestamp + "','" + timestamp + "')",
			"INSERT INTO account_entries(user_id,entry_id,last_seen_at) VALUES('user_1','entry_1','" + timestamp + "')",
			"INSERT INTO topics(topic_id,user_id,name,normalized_name,kind,pinned,hidden,sort_order,created_at,updated_at) VALUES('topic_1','user_1','Topic','topic','core',0,0,10,'" + timestamp + "','" + timestamp + "')",
			"INSERT INTO entry_topics(user_id,entry_id,topic_id,confidence,is_primary,content_hash,created_at) VALUES('user_1','entry_1','topic_1',0.9,1,'" + hash + "','" + timestamp + "')",
			"INSERT INTO daily_queues(queue_id,user_id,local_date,filter_key,timezone,status,version,generated_at,created_at,updated_at) VALUES('queue_1','user_1','2026-08-09','default','Asia/Shanghai','ready',1,'" + timestamp + "','" + timestamp + "','" + timestamp + "')",
			"INSERT INTO daily_queue_items(queue_id,entry_id,rank,score,score_json,state,added_at,updated_at) VALUES('queue_1','entry_1',1,10,'{}','unread','" + timestamp + "','" + timestamp + "')",
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
	service, err := recommendation.NewFeedbackService(recommendation.FeedbackConfig{Store: store, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	request := recommendation.FeedbackRequest{UserID: "user_1", EntryID: "entry_1", Action: "block_source", IdempotencyKey: "feedback-key-0001"}
	const workers = 8
	errorsChannel := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := service.Apply(ctx, request)
			if err != nil {
				errorsChannel <- err
				return
			}
			if !result.Applied {
				errorsChannel <- errors.New("feedback replay was not applied")
			}
		}()
	}
	group.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatal(err)
	}
	var events, blocks int
	var itemState string
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM recommendation_events").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM recommendation_blocks WHERE target_type='source' AND target_id='feed_1'").Scan(&blocks); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT state FROM daily_queue_items WHERE queue_id='queue_1' AND entry_id='entry_1'").Scan(&itemState); err != nil {
		t.Fatal(err)
	}
	if events != 1 || blocks != 1 || itemState != "removed" {
		t.Fatalf("events=%d blocks=%d state=%s", events, blocks, itemState)
	}
	conflict := request
	conflict.Action = "not_interested"
	if _, err := service.Apply(ctx, conflict); !errors.Is(err, recommendation.ErrIdempotencyConflict) {
		t.Fatalf("conflicting replay error=%v", err)
	}
	undo := recommendation.FeedbackRequest{UserID: "user_1", EntryID: "entry_1", Action: "undo", IdempotencyKey: "feedback-key-0002"}
	if result, err := service.Apply(ctx, undo); err != nil || !result.Applied {
		t.Fatalf("undo=%#v err=%v", result, err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM recommendation_blocks WHERE target_type='source' AND target_id='feed_1'").Scan(&blocks); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT state FROM daily_queue_items WHERE queue_id='queue_1' AND entry_id='entry_1'").Scan(&itemState); err != nil {
		t.Fatal(err)
	}
	if blocks != 0 || itemState != "removed" {
		t.Fatalf("undo blocks=%d current queue state=%s", blocks, itemState)
	}
}
