//go:build !race

package home_test

import (
	"context"
	"database/sql"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/home"
)

func TestLoad100KHomeMeetsLatencyAndMemoryBudget(t *testing.T) {
	ctx := context.Background()
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	store := openHomeStore(t)
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	createdAt := now.Add(-time.Hour).Format(time.RFC3339Nano)
	hash := strings.Repeat("c", 64)
	if err := store.Write(ctx, func(transaction *sql.Tx) error {
		if _, err := transaction.ExecContext(ctx, "INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES(?,?,?,?,?)", "user_load", "Load User", "Asia/Shanghai", createdAt, createdAt); err != nil {
			return err
		}
		for index := 0; index < 20; index++ {
			if _, err := transaction.ExecContext(ctx, "INSERT INTO feeds(feed_id,title,view,updated_at) VALUES(?,?,0,?)", fmt.Sprintf("feed_%02d", index), fmt.Sprintf("Source %02d", index), createdAt); err != nil {
				return err
			}
		}
		entryStatement, err := transaction.PrepareContext(ctx, `
INSERT INTO entries(entry_id,feed_id,kind,title,content,media_json,published_at,content_hash,created_at,updated_at)
VALUES(?,?,?,?,?,'[]',?,?,?,?)`)
		if err != nil {
			return err
		}
		defer entryStatement.Close()
		accountStatement, err := transaction.PrepareContext(ctx, "INSERT INTO account_entries(user_id,entry_id,last_seen_at) VALUES('user_load',?,?)")
		if err != nil {
			return err
		}
		defer accountStatement.Close()
		for index := 0; index < 100_000; index++ {
			entryID := fmt.Sprintf("entry_%06d", index)
			publishedAt := now.AddDate(0, 0, -8).Add(-time.Duration(index) * time.Second)
			if index < 500 {
				publishedAt = now.Add(-time.Duration(index) * time.Minute)
			}
			if _, err := entryStatement.ExecContext(ctx, entryID, fmt.Sprintf("feed_%02d", index%20), "article", "Title "+entryID, "Body", publishedAt.Format(time.RFC3339Nano), hash, createdAt, createdAt); err != nil {
				return err
			}
			if _, err := accountStatement.ExecContext(ctx, entryID, createdAt); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("load 100k Home fixture: %v", err)
	}
	service, err := home.NewService(home.Config{Store: store, CursorKey: []byte("load-home-cursor-key-at-least-32!!"), Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	query := home.Query{UserID: "user_load", TopicID: "recommend", Timezone: "Asia/Shanghai", Limit: 20}
	startedAt := time.Now()
	first, err := service.Get(ctx, query)
	coldLatency := time.Since(startedAt)
	if err != nil || len(first.Items) != 20 || first.Queue.Total != 50 {
		t.Fatalf("cold Home items=%d queue=%#v err=%v", len(first.Items), first.Queue, err)
	}
	for index := 0; index < 3; index++ {
		if _, err := service.Get(ctx, query); err != nil {
			t.Fatalf("warm Home: %v", err)
		}
	}
	latencies := make([]time.Duration, 20)
	for index := range latencies {
		startedAt = time.Now()
		page, err := service.Get(ctx, query)
		latencies[index] = time.Since(startedAt)
		if err != nil || len(page.Items) != 20 || page.Queue.ID != first.Queue.ID {
			t.Fatalf("measured Home items=%d queue=%#v err=%v", len(page.Items), page.Queue, err)
		}
	}
	sort.Slice(latencies, func(left, right int) bool { return latencies[left] < latencies[right] })
	p50 := latencies[(len(latencies)*50-1)/100]
	p95 := latencies[(len(latencies)*95-1)/100]
	if p50 > 50*time.Millisecond || p95 > 150*time.Millisecond {
		t.Fatalf("100k Home P50=%s P95=%s cold=%s", p50, p95, coldLatency)
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	var heapGrowth uint64
	if after.HeapSys > before.HeapSys {
		heapGrowth = after.HeapSys - before.HeapSys
	}
	if heapGrowth > 300*1024*1024 {
		t.Fatalf("100k Home heap growth=%d bytes", heapGrowth)
	}
	t.Logf("100k Home P50=%s P95=%s cold=%s heapGrowth=%d", p50, p95, coldLatency, heapGrowth)
}
