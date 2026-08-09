package search

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"tantan.local/tantan-api/internal/storage"
)

type Config struct {
	Store     *storage.Store
	CursorKey []byte
}

type Service struct {
	store     *storage.Store
	indexer   *Indexer
	cursorKey []byte
}

type Query struct {
	UserID string
	Text   string
	Limit  int
	Cursor string
}

type Topic struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Source struct {
	ID     string
	Name   string
	Avatar *string
}

type Item struct {
	EntryID     string
	Kind        string
	Title       string
	Excerpt     *string
	Cover       *string
	Source      Source
	PublishedAt string
	Topics      []Topic
	Translated  bool
	Read        bool
	Collected   bool
	Score       float64
}

type Page struct {
	Items       []Item
	NextCursor  *string
	IndexStatus string
}

func NewService(config Config) (*Service, error) {
	if config.Store == nil {
		return nil, errors.New("search storage is required")
	}
	if len(config.CursorKey) < 32 {
		return nil, errors.New("search cursor key must contain at least 32 bytes")
	}
	key := append([]byte(nil), config.CursorKey...)
	return &Service{store: config.Store, indexer: NewIndexer(config.Store), cursorKey: key}, nil
}

func (service *Service) Search(ctx context.Context, query Query) (Page, error) {
	query.UserID = strings.TrimSpace(query.UserID)
	query.Text = strings.TrimSpace(query.Text)
	if query.UserID == "" || utf8.RuneCountInString(query.Text) < 1 || utf8.RuneCountInString(query.Text) > 200 {
		return Page{}, errors.New("invalid search query")
	}
	if query.Limit == 0 {
		query.Limit = 20
	}
	if query.Limit < 1 || query.Limit > 50 {
		return Page{}, errors.New("invalid search limit")
	}
	queryHash := hashQuery(query.UserID, query.Text)
	var after *cursorPayload
	if query.Cursor != "" {
		payload, err := decodeCursor(service.cursorKey, query.Cursor, queryHash)
		if err != nil {
			return Page{}, err
		}
		after = &payload
	}
	items, err := service.query(ctx, query, after)
	if err != nil {
		return Page{}, err
	}
	var nextCursor *string
	if len(items) > query.Limit {
		items = items[:query.Limit]
		last := items[len(items)-1]
		encoded, err := encodeCursor(service.cursorKey, cursorPayload{
			Version:     1,
			QueryHash:   queryHash,
			Score:       last.Score,
			PublishedAt: last.PublishedAt,
			EntryID:     last.EntryID,
		})
		if err != nil {
			return Page{}, err
		}
		nextCursor = &encoded
	}
	status, err := service.indexer.Status(ctx, query.UserID)
	if err != nil {
		return Page{}, err
	}
	if items == nil {
		items = []Item{}
	}
	return Page{Items: items, NextCursor: nextCursor, IndexStatus: status}, nil
}

func (service *Service) query(ctx context.Context, query Query, after *cursorPayload) ([]Item, error) {
	matchClause := "entry_search MATCH ?"
	matchArgument := `"` + strings.ReplaceAll(query.Text, `"`, `""`) + `"`
	scoreExpression := "bm25(entry_search)"
	if utf8.RuneCountInString(query.Text) < 3 {
		matchClause = `(entry_search.title LIKE ? ESCAPE '\' OR entry_search.translation LIKE ? ESCAPE '\' OR entry_search.content LIKE ? ESCAPE '\' OR entry_search.source LIKE ? ESCAPE '\' OR entry_search.topics LIKE ? ESCAPE '\' OR entry_search.tags LIKE ? ESCAPE '\')`
		matchArgument = "%" + escapeLike(query.Text) + "%"
		scoreExpression = "0.0"
	}
	arguments := []any{query.UserID}
	if utf8.RuneCountInString(query.Text) < 3 {
		for range 6 {
			arguments = append(arguments, matchArgument)
		}
	} else {
		arguments = append(arguments, matchArgument)
	}
	statement := fmt.Sprintf(`
WITH ranked AS (
  SELECT entry_search.entry_id, %s AS score
  FROM entry_search
  WHERE entry_search.user_id=? AND %s
)
SELECT
  e.entry_id,
  e.kind,
  COALESCE((SELECT en.translated_title FROM entry_enrichments en WHERE en.entry_id=e.entry_id AND en.state='ready' AND en.content_hash=e.content_hash AND en.translated_title IS NOT NULL ORDER BY en.updated_at DESC LIMIT 1), e.title),
  COALESCE((SELECT en.translated_content FROM entry_enrichments en WHERE en.entry_id=e.entry_id AND en.state='ready' AND en.content_hash=e.content_hash AND en.translated_content IS NOT NULL ORDER BY en.updated_at DESC LIMIT 1), e.description, e.content, ''),
  e.media_json,
  e.published_at,
  COALESCE(f.feed_id,''),
  COALESCE(f.title,''),
  f.image,
  ae.read_at,
  ae.collected_at,
  COALESCE((SELECT json_group_array(json_object('id',t.topic_id,'name',t.name)) FROM entry_topics et JOIN topics t ON t.topic_id=et.topic_id AND t.user_id=et.user_id WHERE et.user_id=ae.user_id AND et.entry_id=e.entry_id), '[]'),
  EXISTS(SELECT 1 FROM entry_enrichments en WHERE en.entry_id=e.entry_id AND en.state='ready' AND en.content_hash=e.content_hash AND (en.translated_title IS NOT NULL OR en.translated_content IS NOT NULL)),
  ranked.score
FROM ranked
JOIN entries e ON e.entry_id=ranked.entry_id
JOIN account_entries ae ON ae.entry_id=e.entry_id AND ae.user_id=?
LEFT JOIN feeds f ON f.feed_id=e.feed_id`, scoreExpression, matchClause)
	arguments = append(arguments, query.UserID)
	if after != nil {
		statement += `
WHERE ranked.score > ?
   OR (ranked.score = ? AND e.published_at < ?)
   OR (ranked.score = ? AND e.published_at = ? AND e.entry_id > ?)`
		arguments = append(arguments, after.Score, after.Score, after.PublishedAt, after.Score, after.PublishedAt, after.EntryID)
	}
	statement += "\nORDER BY ranked.score ASC, e.published_at DESC, e.entry_id ASC\nLIMIT ?"
	arguments = append(arguments, query.Limit+1)
	rows, err := service.store.DB().QueryContext(ctx, statement, arguments...)
	if err != nil {
		return nil, fmt.Errorf("query search index: %w", err)
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		var item Item
		var excerpt string
		var mediaJSON string
		var sourceImage sql.NullString
		var readAt sql.NullString
		var collectedAt sql.NullString
		var topicsJSON string
		if err := rows.Scan(
			&item.EntryID,
			&item.Kind,
			&item.Title,
			&excerpt,
			&mediaJSON,
			&item.PublishedAt,
			&item.Source.ID,
			&item.Source.Name,
			&sourceImage,
			&readAt,
			&collectedAt,
			&topicsJSON,
			&item.Translated,
			&item.Score,
		); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		if excerpt != "" {
			item.Excerpt = &excerpt
		}
		if sourceImage.Valid {
			item.Source.Avatar = &sourceImage.String
		}
		item.Cover = coverFromMedia(mediaJSON)
		item.Read = readAt.Valid
		item.Collected = collectedAt.Valid
		if err := json.Unmarshal([]byte(topicsJSON), &item.Topics); err != nil {
			return nil, errors.New("search result contains invalid topics")
		}
		if item.Topics == nil {
			item.Topics = []Topic{}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search results: %w", err)
	}
	return items, nil
}

func coverFromMedia(raw string) *string {
	var media []struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(raw), &media); err != nil || len(media) == 0 || media[0].URL == "" {
		return nil
	}
	return &media[0].URL
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}

func hashQuery(userID, text string) string {
	digest := sha256.Sum256([]byte(userID + "\x00" + text))
	return hex.EncodeToString(digest[:])
}
