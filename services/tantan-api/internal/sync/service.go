package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	stdsync "sync"
	"time"

	"tantan.local/tantan-api/internal/jobs"
	"tantan.local/tantan-api/internal/search"
	"tantan.local/tantan-api/internal/storage"
)

type Config struct {
	Store           *storage.Store
	Source          Source
	Now             func() time.Time
	Sleep           func(context.Context, time.Duration) error
	AfterPageCommit func(Checkpoint) error
}

type Service struct {
	store           *storage.Store
	repository      *storage.Repository
	indexer         *search.Indexer
	source          Source
	now             func() time.Time
	sleep           func(context.Context, time.Duration) error
	afterPageCommit func(Checkpoint) error
	locks           stdsync.Map
}

func NewService(config Config) (*Service, error) {
	if config.Store == nil || config.Source == nil {
		return nil, errors.New("sync storage and source are required")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	sleep := config.Sleep
	if sleep == nil {
		sleep = func(ctx context.Context, duration time.Duration) error {
			timer := time.NewTimer(duration)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		}
	}
	return &Service{
		store:           config.Store,
		repository:      storage.NewRepository(config.Store),
		indexer:         search.NewIndexer(config.Store),
		source:          config.Source,
		now:             now,
		sleep:           sleep,
		afterPageCommit: config.AfterPageCommit,
	}, nil
}

func (service *Service) Run(ctx context.Context, account Account, options RunOptions) (Result, error) {
	if strings.TrimSpace(account.ID) == "" || strings.TrimSpace(account.Name) == "" {
		return Result{}, errors.New("sync account is required")
	}
	if account.Timezone == "" {
		account.Timezone = "Asia/Shanghai"
	}
	lockValue, _ := service.locks.LoadOrStore(account.ID, &stdsync.Mutex{})
	lock := lockValue.(*stdsync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	checkpoint, startedAt, err := service.start(ctx, account, options)
	if err != nil {
		return Result{}, err
	}
	feeds, err := jobs.RetryValue(ctx, service.retryPolicy(), func() ([]RemoteFeed, error) {
		return service.source.ListSubscriptions(ctx, account.ID)
	})
	if err != nil {
		service.markFailed(ctx, account.ID, checkpoint, startedAt, "FOLO_UNAVAILABLE")
		return Result{}, err
	}
	if err := service.store.Write(ctx, func(transaction *sql.Tx) error {
		for _, feed := range feeds {
			if err := service.repository.UpsertFeedTx(ctx, transaction, mapFeed(feed, service.now().UTC())); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		service.markFailed(ctx, account.ID, checkpoint, startedAt, "LOCAL_STORAGE_ERROR")
		return Result{}, err
	}

	for {
		page, err := jobs.RetryValue(ctx, service.retryPolicy(), func() ([]RemoteEntry, error) {
			return service.source.ListEntries(ctx, account.ID, PageRequest{
				Limit:           PageSize,
				PublishedAfter:  cloneTime(checkpoint.PublishedAfter),
				PublishedBefore: cloneTime(checkpoint.PublishedBefore),
			})
		})
		if err != nil {
			service.markFailed(ctx, account.ID, checkpoint, startedAt, "FOLO_UNAVAILABLE")
			return Result{}, err
		}
		if len(page) > PageSize {
			return Result{}, errors.New("Folo entry page exceeded the requested limit")
		}
		if len(page) == 0 {
			return service.finish(ctx, account.ID, checkpoint, startedAt)
		}
		sort.SliceStable(page, func(left, right int) bool {
			if page[left].PublishedAt.Equal(page[right].PublishedAt) {
				return page[left].ID < page[right].ID
			}
			return page[left].PublishedAt.After(page[right].PublishedAt)
		})
		contents, missing, err := service.loadContents(ctx, account.ID, page)
		if err != nil {
			service.markFailed(ctx, account.ID, checkpoint, startedAt, "FOLO_UNAVAILABLE")
			return Result{}, err
		}
		now := service.now().UTC()
		nextCheckpoint := checkpoint
		oldest := page[len(page)-1].PublishedAt.UTC()
		nextCheckpoint.PublishedBefore = &oldest
		nextCheckpoint.Processed += len(page)
		nextCheckpoint.Failed += len(missing)
		if err := service.commitPage(ctx, account, page, contents, missing, nextCheckpoint, startedAt, now); err != nil {
			service.markFailed(ctx, account.ID, checkpoint, startedAt, "LOCAL_STORAGE_ERROR")
			return Result{}, err
		}
		checkpoint = nextCheckpoint
		if service.afterPageCommit != nil {
			if err := service.afterPageCommit(checkpoint); err != nil {
				return Result{}, err
			}
		}
		if len(page) < PageSize {
			return service.finish(ctx, account.ID, checkpoint, startedAt)
		}
	}
}

func (service *Service) start(ctx context.Context, account Account, options RunOptions) (Checkpoint, time.Time, error) {
	state, exists, err := service.repository.GetSyncState(ctx, account.ID)
	if err != nil {
		return Checkpoint{}, time.Time{}, err
	}
	var checkpoint Checkpoint
	var startedAt time.Time
	resume := exists && (state.State == "running" || state.State == "failed") && state.CursorJSON != nil
	if resume {
		if err := json.Unmarshal([]byte(*state.CursorJSON), &checkpoint); err != nil {
			return Checkpoint{}, time.Time{}, errors.New("sync checkpoint is invalid")
		}
		if options.Mode == ModeFull && checkpoint.Mode != ModeFull {
			resume = false
		}
	}
	if resume && state.StartedAt != nil {
		startedAt = *state.StartedAt
	} else {
		startedAt = service.now().UTC()
		mode := options.Mode
		if mode == "" {
			mode = ModeAuto
		}
		if mode == ModeAuto || mode == ModeIncremental {
			lastSuccess, err := service.repository.LastSuccessSync(ctx, account.ID)
			if err != nil {
				return Checkpoint{}, time.Time{}, err
			}
			if lastSuccess == nil {
				mode = ModeFull
			} else {
				mode = ModeIncremental
				overlap := lastSuccess.Add(-5 * time.Minute).UTC()
				checkpoint.PublishedAfter = &overlap
			}
		}
		if mode != ModeFull && mode != ModeIncremental {
			return Checkpoint{}, time.Time{}, errors.New("invalid sync mode")
		}
		checkpoint.Mode = mode
	}
	now := service.now().UTC()
	scope := "all"
	cursorJSON, err := json.Marshal(checkpoint)
	if err != nil {
		return Checkpoint{}, time.Time{}, err
	}
	err = service.store.Write(ctx, func(transaction *sql.Tx) error {
		if err := service.repository.UpsertAccountTx(ctx, transaction, storage.Account{
			ID:       account.ID,
			Name:     account.Name,
			Avatar:   account.Avatar,
			Timezone: account.Timezone,
		}, now); err != nil {
			return err
		}
		return service.repository.PutSyncStateTx(ctx, transaction, storage.SyncState{
			UserID:     account.ID,
			State:      "running",
			Scope:      &scope,
			CursorJSON: stringValue(string(cursorJSON)),
			Total:      checkpoint.Processed,
			Processed:  checkpoint.Processed,
			Failed:     checkpoint.Failed,
			StartedAt:  &startedAt,
			UpdatedAt:  now,
		})
	})
	if err != nil {
		return Checkpoint{}, time.Time{}, err
	}
	return checkpoint, startedAt, nil
}

func (service *Service) loadContents(ctx context.Context, userID string, page []RemoteEntry) (map[string]string, []string, error) {
	contents := make(map[string]string, len(page))
	var missing []string
	for start := 0; start < len(page); start += 50 {
		end := start + 50
		if end > len(page) {
			end = len(page)
		}
		ids := make([]string, 0, end-start)
		for _, entry := range page[start:end] {
			ids = append(ids, entry.ID)
		}
		result, err := jobs.RetryValue(ctx, service.retryPolicy(), func() (contentBatch, error) {
			stream, err := service.source.StreamContents(ctx, userID, ids)
			if err != nil {
				return contentBatch{}, err
			}
			defer stream.Close()
			values, missingIDs, _, err := ParseContentStream(stream, ids)
			return contentBatch{contents: values, missing: missingIDs}, err
		})
		if err != nil {
			return nil, nil, err
		}
		for id, content := range result.contents {
			contents[id] = content
		}
		missing = append(missing, result.missing...)
	}
	return contents, missing, nil
}

type contentBatch struct {
	contents map[string]string
	missing  []string
}

func (service *Service) commitPage(ctx context.Context, account Account, page []RemoteEntry, contents map[string]string, missing []string, checkpoint Checkpoint, startedAt, now time.Time) error {
	return service.store.Write(ctx, func(transaction *sql.Tx) error {
		feeds := make(map[string]RemoteFeed, len(page))
		for _, remote := range page {
			feeds[remote.Feed.ID] = remote.Feed
		}
		feedIDs := make([]string, 0, len(feeds))
		for feedID := range feeds {
			feedIDs = append(feedIDs, feedID)
		}
		sort.Strings(feedIDs)
		for _, feedID := range feedIDs {
			if err := service.repository.UpsertFeedTx(ctx, transaction, mapFeed(feeds[feedID], now)); err != nil {
				return err
			}
		}
		entryIDs := make([]string, 0, len(page))
		for _, remote := range page {
			content, contentKnown := contents[remote.ID]
			entry := mapEntry(remote, now)
			if contentKnown {
				entry.Content = &content
				entry.ContentKnown = true
			}
			if _, err := service.repository.UpsertEntryTx(ctx, transaction, entry); err != nil {
				return err
			}
			if err := service.repository.UpsertAccountEntryTx(ctx, transaction, storage.AccountEntry{
				UserID:      account.ID,
				EntryID:     remote.ID,
				Read:        remote.Read,
				CollectedAt: remote.CollectedAt,
				LastSeenAt:  now,
			}); err != nil {
				return err
			}
			entryIDs = append(entryIDs, remote.ID)
		}
		if err := service.indexer.RefreshTx(ctx, transaction, account.ID, entryIDs); err != nil {
			return err
		}
		for start := 0; start < len(missing); start += 50 {
			end := start + 50
			if end > len(missing) {
				end = len(missing)
			}
			if err := jobs.EnqueueContentRetryTx(ctx, transaction, account.ID, missing[start:end], now); err != nil {
				return err
			}
		}
		cursorJSON, err := json.Marshal(checkpoint)
		if err != nil {
			return err
		}
		scope := "all"
		return service.repository.PutSyncStateTx(ctx, transaction, storage.SyncState{
			UserID:     account.ID,
			State:      "running",
			Scope:      &scope,
			CursorJSON: stringValue(string(cursorJSON)),
			Total:      checkpoint.Processed,
			Processed:  checkpoint.Processed,
			Failed:     checkpoint.Failed,
			StartedAt:  &startedAt,
			UpdatedAt:  now,
		})
	})
}

func (service *Service) finish(ctx context.Context, userID string, checkpoint Checkpoint, startedAt time.Time) (Result, error) {
	now := service.now().UTC()
	finishedAt := now
	scope := "all"
	err := service.store.Write(ctx, func(transaction *sql.Tx) error {
		if err := service.repository.SetLastSuccessSyncTx(ctx, transaction, userID, now); err != nil {
			return err
		}
		return service.repository.PutSyncStateTx(ctx, transaction, storage.SyncState{
			UserID:     userID,
			State:      "succeeded",
			Scope:      &scope,
			Total:      checkpoint.Processed,
			Processed:  checkpoint.Processed,
			Failed:     checkpoint.Failed,
			StartedAt:  &startedAt,
			UpdatedAt:  now,
			FinishedAt: &finishedAt,
		})
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Processed: checkpoint.Processed, Failed: checkpoint.Failed}, nil
}

func (service *Service) markFailed(ctx context.Context, userID string, checkpoint Checkpoint, startedAt time.Time, code string) {
	now := service.now().UTC()
	scope := "all"
	cursorJSON, err := json.Marshal(checkpoint)
	if err != nil {
		return
	}
	_ = service.store.Write(ctx, func(transaction *sql.Tx) error {
		return service.repository.PutSyncStateTx(ctx, transaction, storage.SyncState{
			UserID:     userID,
			State:      "failed",
			Scope:      &scope,
			CursorJSON: stringValue(string(cursorJSON)),
			Total:      checkpoint.Processed,
			Processed:  checkpoint.Processed,
			Failed:     checkpoint.Failed,
			ErrorCode:  &code,
			StartedAt:  &startedAt,
			UpdatedAt:  now,
		})
	})
}

func mapFeed(remote RemoteFeed, now time.Time) storage.Feed {
	title := strings.TrimSpace(remote.Title)
	if title == "" {
		title = "Unknown source"
	}
	view := 0
	if remote.View == 1 {
		view = 1
	}
	return storage.Feed{
		ID:        remote.ID,
		Title:     title,
		URL:       optionalString(remote.URL),
		Image:     remote.Image,
		View:      view,
		UpdatedAt: now,
	}
}

func mapEntry(remote RemoteEntry, now time.Time) storage.Entry {
	title := strings.TrimSpace(remote.Title)
	if title == "" {
		title = "Untitled"
	}
	media := remote.MediaJSON
	if !json.Valid(media) {
		media = []byte("[]")
	}
	return storage.Entry{
		ID:          remote.ID,
		FeedID:      remote.Feed.ID,
		Kind:        entryKind(remote.View),
		Title:       title,
		Description: optionalString(remote.Description),
		Author:      optionalString(remote.Author),
		URL:         optionalString(remote.URL),
		Language:    optionalString(remote.Language),
		MediaJSON:   media,
		PublishedAt: remote.PublishedAt.UTC(),
		UpdatedAt:   now,
	}
}

func entryKind(view int) string {
	switch view {
	case 1:
		return "post"
	case 2:
		return "image"
	case 3:
		return "video"
	default:
		return "article"
	}
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func stringValue(value string) *string {
	return &value
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (service *Service) retryPolicy() jobs.RetryPolicy {
	return jobs.RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   250 * time.Millisecond,
		MaxDelay:    time.Second,
		Sleep:       service.sleep,
	}
}
