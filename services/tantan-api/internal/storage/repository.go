package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Repository struct {
	store *Store
}

type Account struct {
	ID              string
	Name            string
	Avatar          *string
	Timezone        string
	LastSuccessSync *time.Time
}

type Feed struct {
	ID        string
	Title     string
	URL       *string
	Image     *string
	View      int
	UpdatedAt time.Time
}

type Entry struct {
	ID           string
	FeedID       string
	Kind         string
	Title        string
	Description  *string
	Content      *string
	ContentKnown bool
	Author       *string
	URL          *string
	Language     *string
	MediaJSON    []byte
	PublishedAt  time.Time
	UpdatedAt    time.Time
}

type AccountEntry struct {
	UserID      string
	EntryID     string
	Read        bool
	CollectedAt *time.Time
	LastSeenAt  time.Time
}

type SyncState struct {
	UserID     string
	State      string
	Scope      *string
	CursorJSON *string
	Total      int
	Processed  int
	Failed     int
	ErrorCode  *string
	StartedAt  *time.Time
	UpdatedAt  time.Time
	FinishedAt *time.Time
}

func NewRepository(store *Store) *Repository {
	return &Repository{store: store}
}

func (repository *Repository) Store() *Store {
	return repository.store
}

func (repository *Repository) UpsertAccountTx(ctx context.Context, transaction *sql.Tx, account Account, now time.Time) error {
	if transaction == nil || account.ID == "" || strings.TrimSpace(account.Name) == "" {
		return errors.New("valid account and transaction are required")
	}
	if account.Timezone == "" {
		account.Timezone = "Asia/Shanghai"
	}
	var avatar any
	if account.Avatar != nil {
		avatar = *account.Avatar
	}
	var lastSuccess any
	if account.LastSuccessSync != nil {
		lastSuccess = formatTime(*account.LastSuccessSync)
	}
	timestamp := formatTime(now)
	_, err := transaction.ExecContext(ctx, `
INSERT INTO accounts(user_id,name,avatar,timezone,last_success_sync_at,created_at,updated_at)
VALUES(?,?,?,?,?,?,?)
ON CONFLICT(user_id) DO UPDATE SET
  name=excluded.name,
  avatar=excluded.avatar,
  timezone=excluded.timezone,
  last_success_sync_at=COALESCE(excluded.last_success_sync_at,accounts.last_success_sync_at),
  updated_at=excluded.updated_at`, account.ID, account.Name, avatar, account.Timezone, lastSuccess, timestamp, timestamp)
	if err != nil {
		return fmt.Errorf("upsert account: %w", err)
	}
	return nil
}

func (repository *Repository) UpsertFeedTx(ctx context.Context, transaction *sql.Tx, feed Feed) error {
	if transaction == nil || feed.ID == "" || strings.TrimSpace(feed.Title) == "" || (feed.View != 0 && feed.View != 1) {
		return errors.New("valid feed and transaction are required")
	}
	_, err := transaction.ExecContext(ctx, `
INSERT INTO feeds(feed_id,title,url,image,view,updated_at)
VALUES(?,?,?,?,?,?)
ON CONFLICT(feed_id) DO UPDATE SET
  title=excluded.title,
  url=excluded.url,
  image=excluded.image,
  view=excluded.view,
  updated_at=excluded.updated_at`, feed.ID, feed.Title, nullableString(feed.URL), nullableString(feed.Image), feed.View, formatTime(feed.UpdatedAt))
	if err != nil {
		return fmt.Errorf("upsert feed: %w", err)
	}
	return nil
}

func (repository *Repository) UpsertEntryTx(ctx context.Context, transaction *sql.Tx, entry Entry) (string, error) {
	if transaction == nil || entry.ID == "" || strings.TrimSpace(entry.Title) == "" || entry.PublishedAt.IsZero() {
		return "", errors.New("valid entry and transaction are required")
	}
	if entry.Kind != "article" && entry.Kind != "post" && entry.Kind != "image" && entry.Kind != "video" {
		return "", errors.New("invalid entry kind")
	}
	if len(entry.MediaJSON) == 0 {
		entry.MediaJSON = []byte("[]")
	}
	if !json.Valid(entry.MediaJSON) {
		return "", errors.New("entry media is not valid JSON")
	}
	var existingContent sql.NullString
	var existingPublished string
	var existingHash string
	err := transaction.QueryRowContext(ctx, "SELECT content,published_at,content_hash FROM entries WHERE entry_id=?", entry.ID).Scan(&existingContent, &existingPublished, &existingHash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("read existing entry: %w", err)
	}
	if err == nil {
		publishedAt, parseErr := time.Parse(time.RFC3339Nano, existingPublished)
		if parseErr != nil {
			return "", errors.New("stored entry has invalid published timestamp")
		}
		if publishedAt.After(entry.PublishedAt) {
			return existingHash, nil
		}
		if !entry.ContentKnown {
			if existingContent.Valid {
				value := existingContent.String
				entry.Content = &value
			} else {
				entry.Content = nil
			}
		}
	}
	contentHash, err := hashEntry(entry)
	if err != nil {
		return "", err
	}
	updatedAt := entry.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	_, err = transaction.ExecContext(ctx, `
INSERT INTO entries(entry_id,feed_id,kind,title,description,content,author,url,language,media_json,published_at,content_hash,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(entry_id) DO UPDATE SET
  feed_id=excluded.feed_id,
  kind=excluded.kind,
  title=excluded.title,
  description=excluded.description,
  content=excluded.content,
  author=excluded.author,
  url=excluded.url,
  language=excluded.language,
  media_json=excluded.media_json,
  published_at=excluded.published_at,
  content_hash=excluded.content_hash,
  updated_at=excluded.updated_at`,
		entry.ID,
		nullableText(entry.FeedID),
		entry.Kind,
		entry.Title,
		nullableString(entry.Description),
		nullableString(entry.Content),
		nullableString(entry.Author),
		nullableString(entry.URL),
		nullableString(entry.Language),
		string(entry.MediaJSON),
		formatTime(entry.PublishedAt),
		contentHash,
		formatTime(updatedAt),
		formatTime(updatedAt),
	)
	if err != nil {
		return "", fmt.Errorf("upsert entry: %w", err)
	}
	return contentHash, nil
}

func (repository *Repository) UpsertAccountEntryTx(ctx context.Context, transaction *sql.Tx, state AccountEntry) error {
	if transaction == nil || state.UserID == "" || state.EntryID == "" || state.LastSeenAt.IsZero() {
		return errors.New("valid account entry and transaction are required")
	}
	var readAt any
	if state.Read {
		readAt = formatTime(state.LastSeenAt)
	}
	var collectedAt any
	if state.CollectedAt != nil {
		collectedAt = formatTime(*state.CollectedAt)
	}
	_, err := transaction.ExecContext(ctx, `
INSERT INTO account_entries(user_id,entry_id,read_at,collected_at,last_seen_at)
VALUES(?,?,?,?,?)
ON CONFLICT(user_id,entry_id) DO UPDATE SET
  read_at=CASE WHEN excluded.read_at IS NULL THEN NULL ELSE COALESCE(account_entries.read_at,excluded.read_at) END,
  collected_at=excluded.collected_at,
  last_seen_at=excluded.last_seen_at`, state.UserID, state.EntryID, readAt, collectedAt, formatTime(state.LastSeenAt))
	if err != nil {
		return fmt.Errorf("upsert account entry: %w", err)
	}
	return nil
}

func (repository *Repository) UpdateEntryContentTx(ctx context.Context, transaction *sql.Tx, entryID, content string, now time.Time) error {
	if transaction == nil || strings.TrimSpace(entryID) == "" {
		return errors.New("entry content transaction and ID are required")
	}
	var entry Entry
	var feedID sql.NullString
	var description sql.NullString
	var author sql.NullString
	var entryURL sql.NullString
	var language sql.NullString
	var mediaJSON string
	var publishedAt string
	err := transaction.QueryRowContext(ctx, `
SELECT entry_id,feed_id,kind,title,description,author,url,language,media_json,published_at
FROM entries WHERE entry_id=?`, entryID).Scan(
		&entry.ID,
		&feedID,
		&entry.Kind,
		&entry.Title,
		&description,
		&author,
		&entryURL,
		&language,
		&mediaJSON,
		&publishedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("content retry entry was not found")
	}
	if err != nil {
		return fmt.Errorf("read content retry entry: %w", err)
	}
	entry.FeedID = feedID.String
	entry.Description = stringPointer(description)
	entry.Author = stringPointer(author)
	entry.URL = stringPointer(entryURL)
	entry.Language = stringPointer(language)
	entry.MediaJSON = []byte(mediaJSON)
	entry.Content = &content
	entry.ContentKnown = true
	entry.UpdatedAt = now.UTC()
	if entry.PublishedAt, err = parseTime(publishedAt); err != nil {
		return err
	}
	_, err = repository.UpsertEntryTx(ctx, transaction, entry)
	return err
}

func (repository *Repository) GetSyncState(ctx context.Context, userID string) (SyncState, bool, error) {
	var state SyncState
	var scope sql.NullString
	var cursor sql.NullString
	var errorCode sql.NullString
	var startedAt sql.NullString
	var updatedAt string
	var finishedAt sql.NullString
	err := repository.store.DB().QueryRowContext(ctx, `
SELECT user_id,state,scope,cursor_json,total,processed,failed,error_code,started_at,updated_at,finished_at
FROM sync_state WHERE user_id=?`, userID).Scan(
		&state.UserID,
		&state.State,
		&scope,
		&cursor,
		&state.Total,
		&state.Processed,
		&state.Failed,
		&errorCode,
		&startedAt,
		&updatedAt,
		&finishedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncState{}, false, nil
	}
	if err != nil {
		return SyncState{}, false, fmt.Errorf("read sync state: %w", err)
	}
	state.Scope = stringPointer(scope)
	state.CursorJSON = stringPointer(cursor)
	state.ErrorCode = stringPointer(errorCode)
	if state.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return SyncState{}, false, err
	}
	if state.StartedAt, err = timePointer(startedAt); err != nil {
		return SyncState{}, false, err
	}
	if state.FinishedAt, err = timePointer(finishedAt); err != nil {
		return SyncState{}, false, err
	}
	return state, true, nil
}

func (repository *Repository) PutSyncStateTx(ctx context.Context, transaction *sql.Tx, state SyncState) error {
	if transaction == nil || state.UserID == "" || state.State == "" || state.UpdatedAt.IsZero() {
		return errors.New("valid sync state and transaction are required")
	}
	_, err := transaction.ExecContext(ctx, `
INSERT INTO sync_state(user_id,state,scope,cursor_json,total,processed,failed,error_code,started_at,updated_at,finished_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(user_id) DO UPDATE SET
  state=excluded.state,
  scope=excluded.scope,
  cursor_json=excluded.cursor_json,
  total=excluded.total,
  processed=excluded.processed,
  failed=excluded.failed,
  error_code=excluded.error_code,
  started_at=excluded.started_at,
  updated_at=excluded.updated_at,
  finished_at=excluded.finished_at`,
		state.UserID,
		state.State,
		nullableString(state.Scope),
		nullableString(state.CursorJSON),
		state.Total,
		state.Processed,
		state.Failed,
		nullableString(state.ErrorCode),
		nullableTime(state.StartedAt),
		formatTime(state.UpdatedAt),
		nullableTime(state.FinishedAt),
	)
	if err != nil {
		return fmt.Errorf("upsert sync state: %w", err)
	}
	return nil
}

func (repository *Repository) LastSuccessSync(ctx context.Context, userID string) (*time.Time, error) {
	var raw sql.NullString
	err := repository.store.DB().QueryRowContext(ctx, "SELECT last_success_sync_at FROM accounts WHERE user_id=?", userID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read last successful sync: %w", err)
	}
	if !raw.Valid {
		return nil, nil
	}
	parsed, err := parseTime(raw.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (repository *Repository) SetLastSuccessSyncTx(ctx context.Context, transaction *sql.Tx, userID string, value time.Time) error {
	result, err := transaction.ExecContext(ctx, "UPDATE accounts SET last_success_sync_at=?,updated_at=? WHERE user_id=?", formatTime(value), formatTime(value), userID)
	if err != nil {
		return fmt.Errorf("update last successful sync: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return errors.New("last successful sync account was not found")
	}
	return nil
}

func hashEntry(entry Entry) (string, error) {
	payload := struct {
		Title       string  `json:"title"`
		Description *string `json:"description"`
		Content     *string `json:"content"`
		Author      *string `json:"author"`
		URL         *string `json:"url"`
		Language    *string `json:"language"`
		Media       any     `json:"media"`
	}{
		Title:       entry.Title,
		Description: entry.Description,
		Content:     entry.Content,
		Author:      entry.Author,
		URL:         entry.URL,
		Language:    entry.Language,
	}
	if err := json.Unmarshal(entry.MediaJSON, &payload.Media); err != nil {
		return "", errors.New("entry media is invalid")
	}
	contents, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode entry hash payload: %w", err)
	}
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:]), nil
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(raw string) (time.Time, error) {
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, errors.New("database contains an invalid timestamp")
	}
	return value, nil
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}

func stringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func timePointer(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
