package contentpool

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"tantan.local/tantan-api/internal/home"
	"tantan.local/tantan-api/internal/storage"
)

var (
	ErrCursorInvalid  = errors.New("content pool cursor is invalid")
	ErrCursorMismatch = errors.New("content pool cursor does not match request")
	identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)
)

type Config struct {
	Store     *storage.Store
	CursorKey []byte
}

type Service struct {
	store     *storage.Store
	cursorKey []byte
}

type Query struct {
	UserID   string
	SourceID string
	Limit    int
	Cursor   string
}

type State struct {
	Total   int `json:"total"`
	Ready   int `json:"ready"`
	Pending int `json:"pending"`
}

type Page struct {
	Items      []home.Card `json:"items"`
	NextCursor *string     `json:"nextCursor"`
	Pool       State       `json:"pool"`
}

type cursorPayload struct {
	Version     int    `json:"v"`
	QueryHash   string `json:"q"`
	PublishedAt string `json:"p"`
	EntryID     string `json:"i"`
}

func NewService(config Config) (*Service, error) {
	if config.Store == nil || len(config.CursorKey) < 32 {
		return nil, errors.New("content pool storage and cursor key are required")
	}
	return &Service{store: config.Store, cursorKey: append([]byte(nil), config.CursorKey...)}, nil
}

func (service *Service) List(ctx context.Context, query Query) (Page, error) {
	query.UserID = strings.TrimSpace(query.UserID)
	query.SourceID = strings.TrimSpace(query.SourceID)
	if !validID(query.UserID) || (query.SourceID != "" && !validID(query.SourceID)) {
		return Page{}, errors.New("valid content pool user and source are required")
	}
	if query.Limit == 0 {
		query.Limit = 20
	}
	if query.Limit < 1 || query.Limit > 50 {
		return Page{}, errors.New("content pool limit must be 1 to 50")
	}
	queryHash := poolQueryHash(query.UserID, query.SourceID)
	var after *cursorPayload
	if query.Cursor != "" {
		decoded, err := service.decodeCursor(query.Cursor)
		if err != nil {
			return Page{}, err
		}
		if decoded.QueryHash != queryHash {
			return Page{}, ErrCursorMismatch
		}
		after = &decoded
	}
	state, err := service.state(ctx, query.UserID, query.SourceID)
	if err != nil {
		return Page{}, err
	}
	items, next, err := service.items(ctx, query, queryHash, after)
	if err != nil {
		return Page{}, err
	}
	return Page{Items: items, NextCursor: next, Pool: state}, nil
}

func (service *Service) state(ctx context.Context, userID, sourceID string) (State, error) {
	arguments := []any{userID}
	sourceClause := ""
	if sourceID != "" {
		sourceClause = " AND e.feed_id=?"
		arguments = append(arguments, sourceID)
	}
	statement := `
SELECT COUNT(*),COALESCE(SUM(CASE WHEN ` + displayReadySQL("e") + ` THEN 1 ELSE 0 END),0)
FROM account_entries ae
JOIN entries e ON e.entry_id=ae.entry_id
JOIN feeds f ON f.feed_id=e.feed_id
WHERE ae.user_id=?` + sourceClause
	var state State
	if err := service.store.DB().QueryRowContext(ctx, statement, arguments...).Scan(&state.Total, &state.Ready); err != nil {
		return State{}, fmt.Errorf("read content pool state: %w", err)
	}
	state.Pending = state.Total - state.Ready
	return state, nil
}

func (service *Service) items(ctx context.Context, query Query, queryHash string, after *cursorPayload) ([]home.Card, *string, error) {
	arguments := []any{query.UserID}
	where := "WHERE ae.user_id=?"
	if query.SourceID != "" {
		where += " AND e.feed_id=?"
		arguments = append(arguments, query.SourceID)
	}
	if after != nil {
		where += " AND (e.published_at<? OR (e.published_at=? AND e.entry_id<?))"
		arguments = append(arguments, after.PublishedAt, after.PublishedAt, after.EntryID)
	}
	statement := `
SELECT e.entry_id,e.kind,
  CASE WHEN lower(COALESCE(e.language,'')) LIKE 'zh%' THEN e.title ELSE
    (SELECT en.translated_title FROM entry_enrichments en
     WHERE en.entry_id=e.entry_id AND en.state='ready' AND en.content_hash=e.content_hash
       AND en.translated_title IS NOT NULL AND en.translated_content IS NOT NULL
     ORDER BY en.updated_at DESC LIMIT 1) END,
  CASE WHEN lower(COALESCE(e.language,'')) LIKE 'zh%' THEN COALESCE(e.description,e.content,'') ELSE
    (SELECT en.translated_content FROM entry_enrichments en
     WHERE en.entry_id=e.entry_id AND en.state='ready' AND en.content_hash=e.content_hash
       AND en.translated_title IS NOT NULL AND en.translated_content IS NOT NULL
     ORDER BY en.updated_at DESC LIMIT 1) END,
  e.media_json,e.published_at,f.feed_id,f.title,f.image,
  COALESCE((SELECT json_group_array(json_object('id',topic_id,'name',name)) FROM (
    SELECT t.topic_id AS topic_id,t.name AS name
    FROM entry_topics et JOIN topics t ON t.topic_id=et.topic_id AND t.user_id=et.user_id
    WHERE et.user_id=ae.user_id AND et.entry_id=e.entry_id AND et.content_hash=e.content_hash
    ORDER BY et.is_primary DESC,et.confidence DESC,t.topic_id LIMIT 10
  )),'[]'),
  lower(COALESCE(e.language,'')) NOT LIKE 'zh%'
FROM account_entries ae
JOIN entries e ON e.entry_id=ae.entry_id
JOIN feeds f ON f.feed_id=e.feed_id
` + where + ` AND (` + displayReadySQL("e") + `)
ORDER BY e.published_at DESC,e.entry_id DESC LIMIT ?`
	arguments = append(arguments, query.Limit+1)
	rows, err := service.store.DB().QueryContext(ctx, statement, arguments...)
	if err != nil {
		return nil, nil, fmt.Errorf("read content pool page: %w", err)
	}
	defer rows.Close()
	items := make([]home.Card, 0, query.Limit+1)
	for rows.Next() {
		var item home.Card
		var excerpt string
		var mediaJSON string
		var avatar sql.NullString
		var topicsJSON string
		if err := rows.Scan(&item.EntryID, &item.Type, &item.Title, &excerpt, &mediaJSON, &item.PublishedAt, &item.Source.ID, &item.Source.Name, &avatar, &topicsJSON, &item.Translated); err != nil {
			return nil, nil, fmt.Errorf("scan content pool item: %w", err)
		}
		if excerpt = truncate(strings.TrimSpace(excerpt), 2000); excerpt != "" {
			item.Excerpt = &excerpt
		}
		if avatar.Valid {
			item.Source.Avatar = safeURL(avatar.String)
		}
		item.Cover = cover(mediaJSON)
		if err := json.Unmarshal([]byte(topicsJSON), &item.Topics); err != nil {
			return nil, nil, errors.New("content pool topics are invalid")
		}
		if item.Topics == nil {
			item.Topics = []home.Topic{}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate content pool page: %w", err)
	}
	var next *string
	if len(items) > query.Limit {
		items = items[:query.Limit]
		last := items[len(items)-1]
		encoded, err := service.encodeCursor(cursorPayload{Version: 1, QueryHash: queryHash, PublishedAt: last.PublishedAt, EntryID: last.EntryID})
		if err != nil {
			return nil, nil, err
		}
		next = &encoded
	}
	return items, next, nil
}

func displayReadySQL(alias string) string {
	return `(
  lower(COALESCE(` + alias + `.language,'')) LIKE 'zh%'
  OR EXISTS(
    SELECT 1 FROM entry_enrichments display_enrichment
    WHERE display_enrichment.entry_id=` + alias + `.entry_id
      AND display_enrichment.state='ready'
      AND display_enrichment.content_hash=` + alias + `.content_hash
      AND display_enrichment.translated_title IS NOT NULL
      AND display_enrichment.translated_content IS NOT NULL
  )
)`
}

func validID(value string) bool {
	return len(value) >= 1 && len(value) <= 128 && identifierPattern.MatchString(value)
}

func poolQueryHash(userID, sourceID string) string {
	digest := sha256.Sum256([]byte(userID + "\x00" + sourceID))
	return hex.EncodeToString(digest[:])
}

func (service *Service) encodeCursor(payload cursorPayload) (string, error) {
	contents, err := json.Marshal(payload)
	if err != nil {
		return "", errors.New("encode content pool cursor")
	}
	mac := hmac.New(sha256.New, service.cursorKey)
	_, _ = mac.Write(contents)
	return base64.RawURLEncoding.EncodeToString(contents) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (service *Service) decodeCursor(raw string) (cursorPayload, error) {
	separator := strings.LastIndexByte(raw, '.')
	if separator < 1 || separator == len(raw)-1 || len(raw) > 2048 {
		return cursorPayload{}, ErrCursorInvalid
	}
	contents, err := base64.RawURLEncoding.DecodeString(raw[:separator])
	if err != nil || base64.RawURLEncoding.EncodeToString(contents) != raw[:separator] {
		return cursorPayload{}, ErrCursorInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(raw[separator+1:])
	if err != nil || base64.RawURLEncoding.EncodeToString(signature) != raw[separator+1:] {
		return cursorPayload{}, ErrCursorInvalid
	}
	mac := hmac.New(sha256.New, service.cursorKey)
	_, _ = mac.Write(contents)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return cursorPayload{}, ErrCursorInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var payload cursorPayload
	if err := decoder.Decode(&payload); err != nil || decoder.Decode(&struct{}{}) != io.EOF || payload.Version != 1 || len(payload.QueryHash) != 64 || payload.PublishedAt == "" || !validID(payload.EntryID) {
		return cursorPayload{}, ErrCursorInvalid
	}
	return payload, nil
}

func cover(raw string) *string {
	var media []struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(raw), &media); err != nil || len(media) == 0 {
		return nil
	}
	return safeURL(media[0].URL)
}

func safeURL(value string) *string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil
	}
	return &value
}

func truncate(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}
