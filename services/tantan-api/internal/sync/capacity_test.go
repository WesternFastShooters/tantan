//go:build !race

package sync_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"runtime"
	"testing"
	"time"

	syncer "tantan.local/tantan-api/internal/sync"
)

func TestLoad100KEntrySyncRecoversAndReconciles(t *testing.T) {
	ctx := context.Background()
	runtime.GC()
	var beforeMemory runtime.MemStats
	runtime.ReadMemStats(&beforeMemory)
	dataDirectory := t.TempDir()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	entries := makeEntries(100_000, now.Add(-time.Hour))
	account := syncer.Account{ID: "user_load", Name: "Load User", Timezone: "Asia/Shanghai"}
	store := openStore(t, dataDirectory)
	commits := 0
	startedAt := time.Now()
	service := newService(t, store, &fakeSource{entries: entries}, func() time.Time { return now }, func(syncer.Checkpoint) error {
		commits++
		if commits == 500 {
			return errInjectedCrash
		}
		return nil
	})
	if _, err := service.Run(ctx, account, syncer.RunOptions{Mode: syncer.ModeFull}); !errors.Is(err, errInjectedCrash) {
		t.Fatalf("capacity crash error=%v", err)
	}
	if countRows(t, store, "entries") != 50_000 {
		t.Fatalf("capacity committed rows=%d", countRows(t, store, "entries"))
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close capacity store: %v", err)
	}

	store = openStore(t, dataDirectory)
	defer store.Close()
	resumedSource := &fakeSource{entries: entries}
	service = newService(t, store, resumedSource, func() time.Time { return now.Add(time.Minute) }, nil)
	result, err := service.Run(ctx, account, syncer.RunOptions{Mode: syncer.ModeFull})
	if err != nil {
		t.Fatalf("resume capacity sync: %v", err)
	}
	if result.Processed != len(entries) || result.Failed != 0 {
		t.Fatalf("capacity result=%#v", result)
	}
	if len(resumedSource.requests) == 0 || resumedSource.requests[0].PublishedBefore == nil || !resumedSource.requests[0].PublishedBefore.Equal(entries[49_999].PublishedAt) {
		t.Fatalf("capacity resume boundary=%#v", resumedSource.requests)
	}
	for table, want := range map[string]int{
		"entries":         100_000,
		"account_entries": 100_000,
		"entry_search":    100_000,
	} {
		if got := countRows(t, store, table); got != want {
			t.Fatalf("%s count=%d want=%d", table, got, want)
		}
	}
	var contentCount, readCount, collectedCount int
	if err := store.DB().QueryRowContext(ctx, `
SELECT
  COUNT(*) FILTER (WHERE e.content IS NOT NULL),
  COUNT(*) FILTER (WHERE ae.read_at IS NOT NULL),
  COUNT(*) FILTER (WHERE ae.collected_at IS NOT NULL)
FROM entries e JOIN account_entries ae ON ae.entry_id=e.entry_id
WHERE ae.user_id=?`, account.ID).Scan(&contentCount, &readCount, &collectedCount); err != nil {
		t.Fatalf("capacity reconciliation counts: %v", err)
	}
	if contentCount != 100_000 || readCount != 50_000 || collectedCount != 33_334 {
		t.Fatalf("capacity content/read/collection=%d/%d/%d", contentCount, readCount, collectedCount)
	}
	sourceDigest := sha256.New()
	for _, entry := range entries {
		_, _ = sourceDigest.Write([]byte(entry.ID))
		_, _ = sourceDigest.Write([]byte{0})
	}
	databaseDigest := sha256.New()
	rows, err := store.DB().QueryContext(ctx, `
SELECT e.entry_id
FROM entries e JOIN account_entries ae ON ae.entry_id=e.entry_id
WHERE ae.user_id=? ORDER BY e.entry_id`, account.ID)
	if err != nil {
		t.Fatalf("query capacity IDs: %v", err)
	}
	for rows.Next() {
		var entryID string
		if err := rows.Scan(&entryID); err != nil {
			_ = rows.Close()
			t.Fatalf("scan capacity ID: %v", err)
		}
		_, _ = databaseDigest.Write([]byte(entryID))
		_, _ = databaseDigest.Write([]byte{0})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		t.Fatalf("iterate capacity IDs: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close capacity ID rows: %v", err)
	}
	if got, want := hex.EncodeToString(databaseDigest.Sum(nil)), hex.EncodeToString(sourceDigest.Sum(nil)); got != want {
		t.Fatalf("capacity ID digest=%s want=%s", got, want)
	}
	if integrity, err := store.Integrity(ctx); err != nil || integrity != "ok" {
		t.Fatalf("capacity integrity=%q err=%v", integrity, err)
	}
	runtime.GC()
	var afterMemory runtime.MemStats
	runtime.ReadMemStats(&afterMemory)
	var heapGrowth uint64
	if afterMemory.HeapSys > beforeMemory.HeapSys {
		heapGrowth = afterMemory.HeapSys - beforeMemory.HeapSys
	}
	if heapGrowth > 300*1024*1024 {
		t.Fatalf("capacity heap growth=%d", heapGrowth)
	}
	t.Logf("100k sync duration=%s heapGrowth=%d digest=%s", time.Since(startedAt), heapGrowth, hex.EncodeToString(databaseDigest.Sum(nil)))
}
