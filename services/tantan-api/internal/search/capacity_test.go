//go:build !race

package search_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/search"
)

func TestHundredThousandEntrySearchMeetsLatencyAndMemoryBudget(t *testing.T) {
	ctx := context.Background()
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	store := openSearchStore(t)
	now := "2026-08-09T10:00:00Z"
	hash := strings.Repeat("b", 64)
	err := store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES(?,?,?,?,?)", "user_1", "Load User", "Asia/Shanghai", now, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO feeds(feed_id,title,view,updated_at) VALUES(?,?,?,?)", "feed_1", "Load Source", 0, now); err != nil {
			return err
		}
		entryStatement, err := tx.PrepareContext(ctx, "INSERT INTO entries(entry_id,feed_id,kind,title,content,media_json,published_at,content_hash,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)")
		if err != nil {
			return err
		}
		defer entryStatement.Close()
		accountStatement, err := tx.PrepareContext(ctx, "INSERT INTO account_entries(user_id,entry_id,last_seen_at) VALUES(?,?,?)")
		if err != nil {
			return err
		}
		defer accountStatement.Close()
		searchStatement, err := tx.PrepareContext(ctx, "INSERT INTO entry_search(entry_id,user_id,title,translation,content,source,topics,tags) VALUES(?,?,?,?,?,?,?,?)")
		if err != nil {
			return err
		}
		defer searchStatement.Close()
		for index := 0; index < 100_000; index++ {
			entryID := fmt.Sprintf("entry_%06d", index)
			title := fmt.Sprintf("ordinary fixed item %06d", index)
			if index%1000 == 0 {
				title = fmt.Sprintf("needleterm fixed item %06d", index)
			}
			publishedAt := fmt.Sprintf("2026-08-%02dT%02d:%02d:00Z", 1+(index/1440)%9, (index/60)%24, index%60)
			if _, err := entryStatement.ExecContext(ctx, entryID, "feed_1", "article", title, "fixed body", "[]", publishedAt, hash, now, now); err != nil {
				return err
			}
			if _, err := accountStatement.ExecContext(ctx, "user_1", entryID, now); err != nil {
				return err
			}
			if _, err := searchStatement.ExecContext(ctx, entryID, "user_1", title, "", "fixed body", "Load Source", "", ""); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("load 100k fixture: %v", err)
	}
	service := newSearchService(t, store)
	for index := 0; index < 3; index++ {
		if _, err := service.Search(ctx, search.Query{UserID: "user_1", Text: "needleterm", Limit: 20}); err != nil {
			t.Fatalf("warm search: %v", err)
		}
	}
	latencies := make([]time.Duration, 20)
	for index := range latencies {
		startedAt := time.Now()
		page, err := service.Search(ctx, search.Query{UserID: "user_1", Text: "needleterm", Limit: 20})
		latencies[index] = time.Since(startedAt)
		if err != nil || len(page.Items) != 20 {
			t.Fatalf("measured search items=%d err=%v", len(page.Items), err)
		}
	}
	sort.Slice(latencies, func(left, right int) bool { return latencies[left] < latencies[right] })
	p95 := latencies[(len(latencies)*95-1)/100]
	if p95 > 300*time.Millisecond {
		t.Fatalf("100k Search P95=%s", p95)
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	var heapGrowth uint64
	if after.HeapSys > before.HeapSys {
		heapGrowth = after.HeapSys - before.HeapSys
	}
	if heapGrowth > 300*1024*1024 {
		t.Fatalf("100k heap growth=%d bytes", heapGrowth)
	}
	databaseInfo, err := os.Stat(store.Path())
	if err != nil {
		t.Fatalf("stat load database: %v", err)
	}
	if databaseInfo.Size() > 5*1024*1024*1024 {
		t.Fatalf("100k database size=%d", databaseInfo.Size())
	}
	t.Logf("100k Search P95=%s heapGrowth=%d databaseBytes=%d", p95, heapGrowth, databaseInfo.Size())
}
