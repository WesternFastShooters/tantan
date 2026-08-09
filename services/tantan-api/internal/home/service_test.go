package home_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/home"
	"tantan.local/tantan-api/internal/storage"
)

func openHomeStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(context.Background(), storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func insertHomeFixture(t *testing.T, store *storage.Store, now time.Time, count int, createdAt time.Time, prefix string) {
	t.Helper()
	ctx := context.Background()
	err := store.Write(ctx, func(transaction *sql.Tx) error {
		timestamp := now.Format(time.RFC3339Nano)
		if prefix == "base" {
			if _, err := transaction.ExecContext(ctx, "INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES(?,?,?,?,?)", "user_1", "Home User", "Asia/Shanghai", timestamp, timestamp); err != nil {
				return err
			}
			for source := 0; source < 12; source++ {
				if _, err := transaction.ExecContext(ctx, "INSERT INTO feeds(feed_id,title,image,view,updated_at) VALUES(?,?,?,?,?)", fmt.Sprintf("feed_%02d", source), fmt.Sprintf("Source %02d", source), fmt.Sprintf("https://image.invalid/%d.png", source), 0, timestamp); err != nil {
					return err
				}
			}
			for topicIndex := 0; topicIndex < 10; topicIndex++ {
				if _, err := transaction.ExecContext(ctx, "INSERT INTO topics(topic_id,user_id,name,normalized_name,kind,pinned,hidden,sort_order,created_at,updated_at) VALUES(?,?,?,?, 'dynamic',0,0,?,?,?)", fmt.Sprintf("topic_%02d", topicIndex), "user_1", fmt.Sprintf("Topic %02d", topicIndex), fmt.Sprintf("topic %02d", topicIndex), (topicIndex+1)*10, timestamp, timestamp); err != nil {
					return err
				}
			}
		}
		for index := 0; index < count; index++ {
			entryID := fmt.Sprintf("%s_%03d", prefix, index)
			published := now.Add(-time.Duration(index) * time.Minute).Format(time.RFC3339Nano)
			hash := fmt.Sprintf("%064x", index+1)
			media := fmt.Sprintf(`[{"url":"https://media.invalid/%s.jpg"}]`, entryID)
			if _, err := transaction.ExecContext(ctx, `
INSERT INTO entries(entry_id,feed_id,kind,title,description,content,language,media_json,published_at,content_hash,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, entryID, fmt.Sprintf("feed_%02d", index%12), []string{"article", "post", "image", "video"}[index%4], "Title "+entryID, "Excerpt "+entryID, "Body "+entryID, "en", media, published, hash, createdAt.Format(time.RFC3339Nano), createdAt.Format(time.RFC3339Nano)); err != nil {
				return err
			}
			var readAt any
			if prefix == "base" && index == count-1 {
				readAt = timestamp
			}
			if _, err := transaction.ExecContext(ctx, "INSERT INTO account_entries(user_id,entry_id,read_at,last_seen_at) VALUES(?,?,?,?)", "user_1", entryID, readAt, timestamp); err != nil {
				return err
			}
			if _, err := transaction.ExecContext(ctx, "INSERT INTO entry_topics(user_id,entry_id,topic_id,confidence,is_primary,content_hash,created_at) VALUES(?,?,?,?,1,?,?)", "user_1", entryID, fmt.Sprintf("topic_%02d", index%10), 0.9, hash, timestamp); err != nil {
				return err
			}
		}
		if prefix == "base" {
			if _, err := transaction.ExecContext(ctx, "INSERT INTO recommendation_blocks(user_id,target_type,target_id,created_at) VALUES(?,?,?,?)", "user_1", "entry", "base_068", timestamp); err != nil {
				return err
			}
			oldTime := now.AddDate(0, 0, -8)
			oldHash := strings.Repeat("f", 64)
			if _, err := transaction.ExecContext(ctx, "INSERT INTO entries(entry_id,feed_id,kind,title,content,media_json,published_at,content_hash,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)", "old_entry", "feed_00", "article", "Old", "old", "[]", oldTime.Format(time.RFC3339Nano), oldHash, oldTime.Format(time.RFC3339Nano), oldTime.Format(time.RFC3339Nano)); err != nil {
				return err
			}
			if _, err := transaction.ExecContext(ctx, "INSERT INTO account_entries(user_id,entry_id,last_seen_at) VALUES(?,?,?)", "user_1", "old_entry", timestamp); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func newHomeService(t *testing.T, store *storage.Store, clock *time.Time) *home.Service {
	t.Helper()
	service, err := home.NewService(home.Config{Store: store, CursorKey: []byte("home-cursor-key-at-least-thirty-two"), Now: func() time.Time { return *clock }})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestDailyQueueIsStableBoundedAndCursorBound(t *testing.T) {
	ctx := context.Background()
	clock := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	store := openHomeStore(t)
	insertHomeFixture(t, store, clock, 70, clock.Add(-time.Hour), "base")
	service := newHomeService(t, store, &clock)
	query := home.Query{UserID: "user_1", TopicID: "recommend", Timezone: "Asia/Shanghai", Limit: 20}
	first, err := service.Get(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.Get(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 20 || first.NextCursor == nil || first.Queue.Total != 50 || first.Queue.Unread != 50 || first.Queue.Finished || first.Queue.CandidateWindowDays != 7 || first.Queue.Generation == "" || first.QueueGeneration != first.Queue.Generation {
		t.Fatalf("first=%#v", first)
	}
	if !reflect.DeepEqual(first.Items, again.Items) || first.Queue.ID != again.Queue.ID || first.Queue.Version != again.Queue.Version {
		t.Fatal("same daily queue was re-ranked or replaced")
	}
	var stored int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM daily_queue_items WHERE queue_id=?", first.Queue.ID).Scan(&stored); err != nil || stored != 50 {
		t.Fatalf("stored=%d err=%v", stored, err)
	}
	second, err := service.Get(ctx, home.Query{UserID: "user_1", TopicID: "recommend", Timezone: "Asia/Shanghai", Limit: 20, Cursor: *first.NextCursor})
	if err != nil || len(second.Items) != 20 || second.NextCursor == nil {
		t.Fatalf("second=%#v err=%v", second, err)
	}
	seen := map[string]bool{}
	for _, item := range append(append([]home.Card{}, first.Items...), second.Items...) {
		if seen[item.EntryID] {
			t.Fatalf("duplicate entry %s", item.EntryID)
		}
		seen[item.EntryID] = true
	}
	if _, err := service.Get(ctx, home.Query{UserID: "user_1", TopicID: "topic_01", Timezone: "Asia/Shanghai", Limit: 20, Cursor: *first.NextCursor}); !errors.Is(err, home.ErrCursorMismatch) {
		t.Fatalf("cross-topic cursor error=%v", err)
	}
	tampered := *first.NextCursor
	if tampered[len(tampered)-1] == 'A' {
		tampered = tampered[:len(tampered)-1] + "B"
	} else {
		tampered = tampered[:len(tampered)-1] + "A"
	}
	if _, err := service.Get(ctx, home.Query{UserID: "user_1", TopicID: "recommend", Timezone: "Asia/Shanghai", Limit: 20, Cursor: tampered}); !errors.Is(err, home.ErrCursorInvalid) {
		t.Fatalf("tampered cursor error=%v", err)
	}
	oldCursor := *first.NextCursor
	rebuilt, err := service.Rebuild(ctx, home.PlanRequest{UserID: "user_1", Timezone: "Asia/Shanghai", FilterKey: "default"})
	if err != nil || rebuilt.ID == first.Queue.ID || rebuilt.Version <= first.Queue.Version {
		t.Fatalf("rebuilt=%#v err=%v", rebuilt, err)
	}
	if _, err := service.Get(ctx, home.Query{UserID: "user_1", TopicID: "recommend", Timezone: "Asia/Shanghai", Limit: 20, Cursor: oldCursor}); !errors.Is(err, home.ErrQueueVersionChanged) {
		t.Fatalf("old-generation cursor error=%v", err)
	}
}

func TestCandidateWindowUsesSevenLocalCalendarDaysAndCapsAtFiveHundred(t *testing.T) {
	t.Run("calendar boundary", func(t *testing.T) {
		ctx := context.Background()
		clock := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
		store := openHomeStore(t)
		insertHomeFixture(t, store, clock, 2, clock.Add(-time.Hour), "base")
		location, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			t.Fatal(err)
		}
		start := time.Date(2026, 8, 3, 0, 0, 0, 0, location).UTC()
		if err := store.Write(ctx, func(transaction *sql.Tx) error {
			for index, item := range []struct {
				id        string
				feed      string
				published time.Time
			}{
				{id: "at_calendar_boundary", feed: "feed_01", published: start},
				{id: "before_calendar_boundary", feed: "feed_02", published: start.Add(-time.Nanosecond)},
				{id: "future_entry", feed: "feed_03", published: clock.Add(time.Second)},
			} {
				hash := fmt.Sprintf("%064x", 10_000+index)
				timestamp := clock.Format(time.RFC3339Nano)
				if _, err := transaction.ExecContext(ctx, `
INSERT INTO entries(entry_id,feed_id,kind,title,content,media_json,published_at,content_hash,created_at,updated_at)
VALUES(?,?,'article',?,'Body','[]',?,?,?,?)`, item.id, item.feed, item.id, item.published.Format(time.RFC3339Nano), hash, timestamp, timestamp); err != nil {
					return err
				}
				if _, err := transaction.ExecContext(ctx, "INSERT INTO account_entries(user_id,entry_id,last_seen_at) VALUES('user_1',?,?)", item.id, timestamp); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		service := newHomeService(t, store, &clock)
		plan, err := service.Plan(ctx, home.PlanRequest{UserID: "user_1", Timezone: "Asia/Shanghai", FilterKey: "default"})
		if err != nil {
			t.Fatal(err)
		}
		selected := make(map[string]bool, len(plan.Items))
		for _, item := range plan.Items {
			selected[item.EntryID] = true
		}
		if !selected["at_calendar_boundary"] || selected["before_calendar_boundary"] || selected["future_entry"] {
			t.Fatalf("calendar candidates=%v", selected)
		}
	})

	t.Run("latest five hundred", func(t *testing.T) {
		ctx := context.Background()
		clock := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
		store := openHomeStore(t)
		insertHomeFixture(t, store, clock, 501, clock.Add(-time.Hour), "base")
		if err := store.Write(ctx, func(transaction *sql.Tx) error {
			if _, err := transaction.ExecContext(ctx, "UPDATE account_entries SET read_at=NULL WHERE user_id='user_1' AND entry_id='base_500'"); err != nil {
				return err
			}
			if _, err := transaction.ExecContext(ctx, "DELETE FROM recommendation_blocks WHERE user_id='user_1'"); err != nil {
				return err
			}
			var hash string
			if err := transaction.QueryRowContext(ctx, "SELECT content_hash FROM entries WHERE entry_id='base_500'").Scan(&hash); err != nil {
				return err
			}
			timestamp := clock.Format(time.RFC3339Nano)
			_, err := transaction.ExecContext(ctx, `
INSERT INTO entry_enrichments(entry_id,provider_fp,language,state,quality_score,content_hash,prompt_version,created_at,updated_at)
VALUES('base_500','123456789012','en','ready',15,?,'prompt-v1',?,?)`, hash, timestamp, timestamp)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		service := newHomeService(t, store, &clock)
		plan, err := service.Plan(ctx, home.PlanRequest{UserID: "user_1", Timezone: "Asia/Shanghai", FilterKey: "default"})
		if err != nil || len(plan.Items) != 50 {
			t.Fatalf("items=%d err=%v", len(plan.Items), err)
		}
		for _, item := range plan.Items {
			if item.EntryID == "base_500" {
				t.Fatal("candidate outside the latest 500 was ranked")
			}
		}
	})
}

func TestSameDayAppendHonorsExistingSourceTail(t *testing.T) {
	ctx := context.Background()
	clock := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	store := openHomeStore(t)
	insertHomeFixture(t, store, clock, 55, clock.Add(-time.Hour), "base")
	service := newHomeService(t, store, &clock)
	first, err := service.Get(ctx, home.Query{UserID: "user_1", TopicID: "recommend", Timezone: "Asia/Shanghai", Limit: 50})
	if err != nil || first.Queue.Total != 50 {
		t.Fatalf("first=%#v err=%v", first.Queue, err)
	}
	if err := store.Write(ctx, func(transaction *sql.Tx) error {
		_, err := transaction.ExecContext(ctx, `
UPDATE entries SET feed_id='feed_00'
WHERE entry_id IN (
  SELECT entry_id FROM daily_queue_items WHERE queue_id=? AND rank IN (49,50)
)`, first.Queue.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Hour)
	insertHomeFixture(t, store, clock, 3, clock, "tail")
	if _, err := service.Get(ctx, home.Query{UserID: "user_1", TopicID: "recommend", Timezone: "Asia/Shanghai", Limit: 50}); err != nil {
		t.Fatal(err)
	}
	rows, err := store.DB().QueryContext(ctx, `
SELECT e.feed_id FROM daily_queue_items qi
JOIN entries e ON e.entry_id=qi.entry_id
WHERE qi.queue_id=? AND qi.rank>=49 ORDER BY qi.rank`, first.Queue.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var sources []string
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for index := 2; index < len(sources); index++ {
		if sources[index-2] == sources[index-1] && sources[index-1] == sources[index] {
			t.Fatalf("same-day append created three consecutive sources: %v", sources)
		}
	}
}

func TestSameDayAppendStopsAtSixtyAndMidnightCreatesNewQueue(t *testing.T) {
	ctx := context.Background()
	clock := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	store := openHomeStore(t)
	insertHomeFixture(t, store, clock, 55, clock.Add(-time.Hour), "base")
	service := newHomeService(t, store, &clock)
	first, err := service.Get(ctx, home.Query{UserID: "user_1", TopicID: "recommend", Timezone: "Asia/Shanghai", Limit: 50})
	if err != nil || first.Queue.Total != 50 {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	clock = clock.Add(time.Hour)
	insertHomeFixture(t, store, clock, 15, clock, "new")
	appended, err := service.Get(ctx, home.Query{UserID: "user_1", TopicID: "recommend", Timezone: "Asia/Shanghai", Limit: 50})
	if err != nil || appended.Queue.ID != first.Queue.ID || appended.Queue.Version != first.Queue.Version || appended.Queue.Total != 60 {
		t.Fatalf("appended=%#v err=%v", appended.Queue, err)
	}
	clock = clock.Add(13 * time.Hour)
	nextDay, err := service.Get(ctx, home.Query{UserID: "user_1", TopicID: "recommend", Timezone: "Asia/Shanghai", Limit: 50})
	if err != nil || nextDay.Queue.ID == first.Queue.ID || nextDay.Queue.Total > 50 {
		t.Fatalf("nextDay=%#v err=%v", nextDay.Queue, err)
	}
}

func TestTopicIsAStableQueueViewAndFullConsumptionFinishesToday(t *testing.T) {
	ctx := context.Background()
	clock := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	store := openHomeStore(t)
	insertHomeFixture(t, store, clock, 60, clock.Add(-time.Hour), "base")
	service := newHomeService(t, store, &clock)
	recommend, err := service.Get(ctx, home.Query{UserID: "user_1", TopicID: "recommend", Timezone: "Asia/Shanghai", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	topicPage, err := service.Get(ctx, home.Query{UserID: "user_1", TopicID: "topic_01", Timezone: "Asia/Shanghai", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if topicPage.Queue.ID != recommend.Queue.ID || topicPage.Queue.Version != recommend.Queue.Version || topicPage.Queue.Total == 0 || topicPage.Queue.Total >= recommend.Queue.Total {
		t.Fatalf("topic queue=%#v recommend=%#v", topicPage.Queue, recommend.Queue)
	}
	if err := store.Write(ctx, func(transaction *sql.Tx) error {
		_, err := transaction.ExecContext(ctx, `
UPDATE account_entries SET read_at=?
WHERE user_id='user_1' AND entry_id IN (SELECT entry_id FROM daily_queue_items WHERE queue_id=?)`, clock.Format(time.RFC3339Nano), recommend.Queue.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	finished, err := service.Get(ctx, home.Query{UserID: "user_1", TopicID: "recommend", Timezone: "Asia/Shanghai", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if finished.Queue.ID != recommend.Queue.ID || !finished.Queue.Finished || finished.Queue.Unread != 0 || finished.Queue.Total != recommend.Queue.Total || len(finished.Items) != 0 {
		t.Fatalf("finished=%#v", finished)
	}
	var historical int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM account_entries WHERE user_id='user_1' AND read_at IS NOT NULL").Scan(&historical); err != nil || historical < recommend.Queue.Total {
		t.Fatalf("historical=%d err=%v", historical, err)
	}
}

func TestConcurrentFirstHomeRequestPublishesOneReadyQueue(t *testing.T) {
	ctx := context.Background()
	clock := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	store := openHomeStore(t)
	insertHomeFixture(t, store, clock, 60, clock.Add(-time.Hour), "base")
	service := newHomeService(t, store, &clock)
	const workers = 20
	type identity struct{ id, generation string }
	identities := make(chan identity, workers)
	errorsChannel := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			page, err := service.Get(ctx, home.Query{UserID: "user_1", TopicID: "recommend", Timezone: "Asia/Shanghai", Limit: 20})
			if err != nil {
				errorsChannel <- err
				return
			}
			identities <- identity{id: page.Queue.ID, generation: page.Queue.Generation}
		}()
	}
	group.Wait()
	close(identities)
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatal(err)
	}
	first := ""
	firstGeneration := ""
	for item := range identities {
		if first == "" {
			first = item.id
			firstGeneration = item.generation
		}
		if item.id != first || item.generation == "" || item.generation != firstGeneration {
			t.Fatalf("concurrent queue identities %q/%q and %q/%q", first, firstGeneration, item.id, item.generation)
		}
	}
	var ready int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM daily_queues WHERE user_id='user_1' AND local_date='2026-08-09' AND filter_key='default' AND status='ready'").Scan(&ready); err != nil || ready != 1 {
		t.Fatalf("ready=%d err=%v", ready, err)
	}
}
