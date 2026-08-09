package sync_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/storage"
	syncer "tantan.local/tantan-api/internal/sync"
)

var errInjectedCrash = errors.New("injected process crash")

type fakeSource struct {
	entries      []syncer.RemoteEntry
	requests     []syncer.PageRequest
	contentLimit int
	invalidLines int
}

func (source *fakeSource) ListSubscriptions(context.Context, string) ([]syncer.RemoteFeed, error) {
	return []syncer.RemoteFeed{{ID: "feed_1", Title: "Source One", URL: "https://source.invalid/feed", View: 0}}, nil
}

func (source *fakeSource) ListEntries(_ context.Context, _ string, request syncer.PageRequest) ([]syncer.RemoteEntry, error) {
	source.requests = append(source.requests, request)
	start := 0
	if request.PublishedBefore != nil {
		start = sort.Search(len(source.entries), func(index int) bool {
			return source.entries[index].PublishedAt.Before(*request.PublishedBefore)
		})
	}
	var page []syncer.RemoteEntry
	for _, entry := range source.entries[start:] {
		if request.PublishedAfter != nil && !entry.PublishedAt.After(*request.PublishedAfter) {
			break
		}
		page = append(page, entry)
		if len(page) == request.Limit {
			break
		}
	}
	return page, nil
}

func (source *fakeSource) StreamContents(_ context.Context, _ string, ids []string) (io.ReadCloser, error) {
	limit := len(ids)
	if source.contentLimit > 0 && source.contentLimit < limit {
		limit = source.contentLimit
	}
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	for _, id := range ids[:limit] {
		if err := encoder.Encode(map[string]string{"id": id, "content": "content for " + id}); err != nil {
			return nil, err
		}
	}
	for index := 0; index < source.invalidLines; index++ {
		_, _ = fmt.Fprintf(buffer, "{invalid-json-%d}\n", index)
	}
	return io.NopCloser(buffer), nil
}

func makeEntries(count int, newest time.Time) []syncer.RemoteEntry {
	entries := make([]syncer.RemoteEntry, 0, count)
	for index := 0; index < count; index++ {
		publishedAt := newest.Add(-time.Duration(index) * time.Minute)
		collectedAt := (*time.Time)(nil)
		if index%3 == 0 {
			value := publishedAt.Add(time.Hour)
			collectedAt = &value
		}
		entries = append(entries, syncer.RemoteEntry{
			ID:          fmt.Sprintf("entry_%06d", index),
			Feed:        syncer.RemoteFeed{ID: "feed_1", Title: "Source One", URL: "https://source.invalid/feed", View: index % 4},
			View:        index % 4,
			Title:       fmt.Sprintf("Entry %06d", index),
			Description: "description",
			Author:      "Author",
			URL:         fmt.Sprintf("https://content.invalid/%d", index),
			Language:    "en",
			MediaJSON:   []byte("[]"),
			PublishedAt: publishedAt,
			Read:        index%2 == 0,
			CollectedAt: collectedAt,
		})
	}
	return entries
}

func openStore(t *testing.T, dataDirectory string) *storage.Store {
	t.Helper()
	store, err := storage.Open(context.Background(), storage.Config{DataDir: dataDirectory})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	return store
}

func newService(t *testing.T, store *storage.Store, source syncer.Source, now func() time.Time, hook func(syncer.Checkpoint) error) *syncer.Service {
	t.Helper()
	service, err := syncer.NewService(syncer.Config{Store: store, Source: source, Now: now, AfterPageCommit: hook})
	if err != nil {
		t.Fatalf("create sync service: %v", err)
	}
	return service
}

func countRows(t *testing.T, store *storage.Store, table string) int {
	t.Helper()
	var count int
	if err := store.DB().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func TestFullSyncIsPageBoundedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	dataDirectory := t.TempDir()
	store := openStore(t, dataDirectory)
	defer store.Close()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	source := &fakeSource{entries: makeEntries(250, now.Add(-time.Hour))}
	service := newService(t, store, source, func() time.Time { return now }, nil)
	account := syncer.Account{ID: "user_1", Name: "Test User", Timezone: "Asia/Shanghai"}

	for run := 0; run < 2; run++ {
		result, err := service.Run(ctx, account, syncer.RunOptions{Mode: syncer.ModeFull})
		if err != nil {
			t.Fatalf("sync run %d: %v", run+1, err)
		}
		if result.Processed != 250 || result.Failed != 0 {
			t.Fatalf("sync result=%#v", result)
		}
		if countRows(t, store, "entries") != 250 || countRows(t, store, "account_entries") != 250 {
			t.Fatal("full sync counts differ from source")
		}
	}
	if countRows(t, store, "entries") != 250 {
		t.Fatal("second full sync created duplicate entries")
	}
	if len(source.requests) != 6 {
		t.Fatalf("entry page request count=%d", len(source.requests))
	}
	for _, firstRequestIndex := range []int{0, 3} {
		if source.requests[firstRequestIndex].Limit != 100 || source.requests[firstRequestIndex].PublishedBefore != nil {
			t.Fatalf("first page request=%#v", source.requests[firstRequestIndex])
		}
		if got := source.requests[firstRequestIndex+1].PublishedBefore; got == nil || !got.Equal(source.entries[99].PublishedAt) {
			t.Fatalf("second page boundary=%v", got)
		}
	}
	if countRows(t, store, "entry_search") != 250 {
		t.Fatal("sync did not maintain explicit FTS rows")
	}
}

func TestSyncResumesFromCommittedCheckpointAfterCrash(t *testing.T) {
	ctx := context.Background()
	dataDirectory := t.TempDir()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	entries := makeEntries(250, now.Add(-time.Hour))
	store := openStore(t, dataDirectory)
	commits := 0
	crashingSource := &fakeSource{entries: entries}
	service := newService(t, store, crashingSource, func() time.Time { return now }, func(syncer.Checkpoint) error {
		commits++
		if commits == 2 {
			return errInjectedCrash
		}
		return nil
	})
	account := syncer.Account{ID: "user_1", Name: "Test User", Timezone: "Asia/Shanghai"}
	_, err := service.Run(ctx, account, syncer.RunOptions{Mode: syncer.ModeFull})
	if !errors.Is(err, errInjectedCrash) {
		t.Fatalf("crash error=%v", err)
	}
	if countRows(t, store, "entries") != 200 {
		t.Fatalf("committed entry count=%d", countRows(t, store, "entries"))
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close crashed store: %v", err)
	}

	store = openStore(t, dataDirectory)
	defer store.Close()
	resumedSource := &fakeSource{entries: entries}
	service = newService(t, store, resumedSource, func() time.Time { return now.Add(time.Minute) }, nil)
	result, err := service.Run(ctx, account, syncer.RunOptions{Mode: syncer.ModeFull})
	if err != nil {
		t.Fatalf("resume sync: %v", err)
	}
	if result.Processed != 250 || countRows(t, store, "entries") != 250 {
		t.Fatalf("resume result=%#v entries=%d", result, countRows(t, store, "entries"))
	}
	if len(resumedSource.requests) != 1 || resumedSource.requests[0].PublishedBefore == nil || !resumedSource.requests[0].PublishedBefore.Equal(entries[199].PublishedAt) {
		t.Fatalf("resume request=%#v", resumedSource.requests)
	}
	if integrity, err := store.Integrity(ctx); err != nil || integrity != "ok" {
		t.Fatalf("integrity=%q err=%v", integrity, err)
	}
}

func TestIncrementalSyncUsesFiveMinuteOverlapWithoutDuplicates(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, t.TempDir())
	defer store.Close()
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	initialEntries := makeEntries(10, now.Add(-time.Minute))
	source := &fakeSource{entries: initialEntries}
	service := newService(t, store, source, func() time.Time { return now }, nil)
	account := syncer.Account{ID: "user_1", Name: "Test User", Timezone: "Asia/Shanghai"}
	if _, err := service.Run(ctx, account, syncer.RunOptions{Mode: syncer.ModeFull}); err != nil {
		t.Fatalf("full sync: %v", err)
	}

	now = now.Add(10 * time.Minute)
	newEntry := makeEntries(1, now.Add(-5*time.Minute))[0]
	newEntry.ID = "entry_new"
	source.entries = append([]syncer.RemoteEntry{newEntry}, initialEntries...)
	source.requests = nil
	if _, err := service.Run(ctx, account, syncer.RunOptions{Mode: syncer.ModeAuto}); err != nil {
		t.Fatalf("incremental sync: %v", err)
	}
	if len(source.requests) != 1 || source.requests[0].PublishedAfter == nil {
		t.Fatalf("incremental requests=%#v", source.requests)
	}
	wantAfter := time.Date(2026, 8, 9, 9, 55, 0, 0, time.UTC)
	if !source.requests[0].PublishedAfter.Equal(wantAfter) {
		t.Fatalf("publishedAfter=%s want=%s", source.requests[0].PublishedAfter, wantAfter)
	}
	if countRows(t, store, "entries") != 11 {
		t.Fatalf("incremental entry count=%d", countRows(t, store, "entries"))
	}
}

func TestExplicitIncrementalSyncUsesLastSuccessCheckpoint(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, t.TempDir())
	defer store.Close()
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	source := &fakeSource{entries: makeEntries(3, now.Add(-time.Minute))}
	service := newService(t, store, source, func() time.Time { return now }, nil)
	account := syncer.Account{ID: "user_1", Name: "Test User", Timezone: "Asia/Shanghai"}
	if _, err := service.Run(ctx, account, syncer.RunOptions{Mode: syncer.ModeFull}); err != nil {
		t.Fatalf("full sync: %v", err)
	}

	now = now.Add(15 * time.Minute)
	source.requests = nil
	if _, err := service.Run(ctx, account, syncer.RunOptions{Mode: syncer.ModeIncremental}); err != nil {
		t.Fatalf("explicit incremental sync: %v", err)
	}
	if len(source.requests) != 1 || source.requests[0].PublishedAfter == nil {
		t.Fatalf("incremental requests=%#v", source.requests)
	}
	want := time.Date(2026, 8, 9, 9, 55, 0, 0, time.UTC)
	if !source.requests[0].PublishedAfter.Equal(want) {
		t.Fatalf("publishedAfter=%s want=%s", source.requests[0].PublishedAfter, want)
	}
}

func TestNDJSONPartialFailureCommitsSuccessesAndQueuesOnlyMissingIDs(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, t.TempDir())
	defer store.Close()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	source := &fakeSource{entries: makeEntries(50, now.Add(-time.Hour)), contentLimit: 45, invalidLines: 2}
	service := newService(t, store, source, func() time.Time { return now }, nil)
	result, err := service.Run(ctx, syncer.Account{ID: "user_1", Name: "Test User", Timezone: "Asia/Shanghai"}, syncer.RunOptions{Mode: syncer.ModeFull})
	if err != nil {
		t.Fatalf("sync partial content: %v", err)
	}
	if result.Processed != 50 || result.Failed != 5 {
		t.Fatalf("partial result=%#v", result)
	}
	var withContent int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM entries WHERE content IS NOT NULL AND content <> ''").Scan(&withContent); err != nil {
		t.Fatalf("count content: %v", err)
	}
	if withContent != 45 || countRows(t, store, "entries") != 50 {
		t.Fatalf("entries=%d withContent=%d", countRows(t, store, "entries"), withContent)
	}
	var retryJobs int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE kind = 'content' AND state = 'queued'").Scan(&retryJobs); err != nil {
		t.Fatalf("count retry jobs: %v", err)
	}
	if retryJobs != 1 {
		t.Fatalf("content retry jobs=%d", retryJobs)
	}
	var payload string
	if err := store.DB().QueryRowContext(ctx, "SELECT payload_json FROM jobs WHERE kind = 'content'").Scan(&payload); err != nil {
		t.Fatalf("read retry payload: %v", err)
	}
	var retryPayload struct {
		EntryIDs []string `json:"entryIds"`
	}
	if err := json.Unmarshal([]byte(payload), &retryPayload); err != nil {
		t.Fatalf("decode retry payload: %v", err)
	}
	sort.Strings(retryPayload.EntryIDs)
	if len(retryPayload.EntryIDs) != 5 {
		t.Fatalf("retry IDs=%v", retryPayload.EntryIDs)
	}

	source.contentLimit = 0
	source.invalidLines = 0
	ran, err := service.RunOneContentJob(ctx)
	if err != nil || !ran {
		t.Fatalf("run content retry ran=%t err=%v", ran, err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM entries WHERE content IS NOT NULL AND content <> ''").Scan(&withContent); err != nil {
		t.Fatalf("count retried content: %v", err)
	}
	if withContent != 50 {
		t.Fatalf("retried content count=%d", withContent)
	}
	var succeededJobs int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE kind='content' AND state='succeeded'").Scan(&succeededJobs); err != nil {
		t.Fatalf("count succeeded content jobs: %v", err)
	}
	if succeededJobs != 1 {
		t.Fatalf("succeeded content jobs=%d", succeededJobs)
	}
}
