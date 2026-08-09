package home

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

func (service *Service) readPage(ctx context.Context, userID, topicID string, queue queueRecord, limit, afterRank int, queryHash string) ([]Card, *string, QueueState, error) {
	state, err := service.queueState(ctx, queue, topicID)
	if err != nil {
		return nil, nil, QueueState{}, err
	}
	topicClause := ""
	arguments := []any{userID, queue.ID, afterRank}
	if topicID != "recommend" {
		topicClause = `
  AND EXISTS(
    SELECT 1 FROM entry_topics selected_topic
    WHERE selected_topic.user_id=ae.user_id AND selected_topic.entry_id=e.entry_id
      AND selected_topic.topic_id=? AND selected_topic.content_hash=e.content_hash
  )`
		arguments = append(arguments, topicID)
	}
	statement := `
SELECT
  qi.rank,e.entry_id,e.kind,
  COALESCE((SELECT en.translated_title FROM entry_enrichments en
    WHERE en.entry_id=e.entry_id AND en.state='ready' AND en.content_hash=e.content_hash AND en.translated_title IS NOT NULL
    ORDER BY en.updated_at DESC LIMIT 1),e.title),
  COALESCE((SELECT en.translated_content FROM entry_enrichments en
    WHERE en.entry_id=e.entry_id AND en.state='ready' AND en.content_hash=e.content_hash AND en.translated_content IS NOT NULL
    ORDER BY en.updated_at DESC LIMIT 1),e.description,e.content,''),
  e.media_json,e.published_at,f.feed_id,f.title,f.image,
  COALESCE((SELECT json_group_array(json_object('id',topic_id,'name',name)) FROM (
    SELECT t.topic_id AS topic_id,t.name AS name
    FROM entry_topics et JOIN topics t ON t.topic_id=et.topic_id AND t.user_id=et.user_id
    WHERE et.user_id=ae.user_id AND et.entry_id=e.entry_id AND et.content_hash=e.content_hash
    ORDER BY et.is_primary DESC,et.confidence DESC,t.topic_id LIMIT 10
  )),'[]'),
  EXISTS(SELECT 1 FROM entry_enrichments en
    WHERE en.entry_id=e.entry_id AND en.state='ready' AND en.content_hash=e.content_hash
      AND (en.translated_title IS NOT NULL OR en.translated_content IS NOT NULL))
FROM daily_queue_items qi
JOIN entries e ON e.entry_id=qi.entry_id
JOIN account_entries ae ON ae.entry_id=e.entry_id AND ae.user_id=?
JOIN feeds f ON f.feed_id=e.feed_id
WHERE qi.queue_id=? AND qi.rank>? AND qi.state='unread' AND ae.read_at IS NULL` + topicClause + `
ORDER BY qi.rank LIMIT ?`
	arguments = append(arguments, limit+1)
	rows, err := service.store.DB().QueryContext(ctx, statement, arguments...)
	if err != nil {
		return nil, nil, QueueState{}, fmt.Errorf("read home page: %w", err)
	}
	defer rows.Close()
	items := make([]Card, 0, limit+1)
	for rows.Next() {
		var card Card
		var excerpt string
		var mediaJSON string
		var avatar sql.NullString
		var topicsJSON string
		if err := rows.Scan(&card.Rank, &card.EntryID, &card.Type, &card.Title, &excerpt, &mediaJSON, &card.PublishedAt, &card.Source.ID, &card.Source.Name, &avatar, &topicsJSON, &card.Translated); err != nil {
			return nil, nil, QueueState{}, fmt.Errorf("scan home card: %w", err)
		}
		if excerpt = truncateRunes(strings.TrimSpace(excerpt), 2000); excerpt != "" {
			card.Excerpt = &excerpt
		}
		if avatar.Valid {
			card.Source.Avatar = safeHTTPURL(avatar.String)
		}
		card.Cover = coverFromMedia(mediaJSON)
		if err := json.Unmarshal([]byte(topicsJSON), &card.Topics); err != nil {
			return nil, nil, QueueState{}, errors.New("home card contains invalid topics")
		}
		if card.Topics == nil {
			card.Topics = []Topic{}
		}
		items = append(items, card)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, QueueState{}, fmt.Errorf("iterate home page: %w", err)
	}
	var next *string
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		encoded, err := encodeCursor(service.cursorKey, cursorPayload{Version: 1, QueryHash: queryHash, QueueID: queue.ID, QueueVer: queue.Version, AfterRank: last.Rank})
		if err != nil {
			return nil, nil, QueueState{}, err
		}
		next = &encoded
	}
	return items, next, state, nil
}

func (service *Service) queueState(ctx context.Context, queue queueRecord, topicID string) (QueueState, error) {
	topicClause := ""
	arguments := []any{queue.UserID, queue.ID}
	if topicID != "recommend" {
		topicClause = ` AND EXISTS(
  SELECT 1 FROM entries e JOIN entry_topics et ON et.entry_id=e.entry_id
  WHERE e.entry_id=qi.entry_id AND et.user_id=ae.user_id AND et.topic_id=? AND et.content_hash=e.content_hash
)`
		arguments = append(arguments, topicID)
	}
	statement := `
SELECT
  COUNT(*) FILTER (WHERE qi.state<>'removed'),
  COUNT(*) FILTER (WHERE qi.state='unread' AND ae.read_at IS NULL)
FROM daily_queue_items qi
JOIN account_entries ae ON ae.entry_id=qi.entry_id AND ae.user_id=?
WHERE qi.queue_id=?`
	statement += topicClause
	var total int
	var unread int
	if err := service.store.DB().QueryRowContext(ctx, statement, arguments...).Scan(&total, &unread); err != nil {
		return QueueState{}, fmt.Errorf("read home queue state: %w", err)
	}
	return QueueState{ID: queue.ID, Version: queue.Version, Total: total, Unread: unread, Finished: unread == 0, CandidateWindowDays: 7, GeneratedAt: queue.GeneratedAt.UTC().Format(time.RFC3339Nano)}, nil
}

func queueStateTx(ctx context.Context, transaction *sql.Tx, queue queueRecord, topicID string) (QueueState, error) {
	if topicID != "recommend" {
		return QueueState{}, errors.New("transactional queue state supports recommend only")
	}
	var total int
	var unread int
	if err := transaction.QueryRowContext(ctx, `
SELECT COUNT(*) FILTER (WHERE qi.state<>'removed'),COUNT(*) FILTER (WHERE qi.state='unread' AND ae.read_at IS NULL)
FROM daily_queue_items qi
JOIN account_entries ae ON ae.entry_id=qi.entry_id AND ae.user_id=?
WHERE qi.queue_id=?`, queue.UserID, queue.ID).Scan(&total, &unread); err != nil {
		return QueueState{}, err
	}
	return QueueState{ID: queue.ID, Version: queue.Version, Total: total, Unread: unread, Finished: unread == 0, CandidateWindowDays: 7, GeneratedAt: queue.GeneratedAt.UTC().Format(time.RFC3339Nano)}, nil
}

func coverFromMedia(raw string) *string {
	var media []struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(raw), &media); err != nil || len(media) == 0 {
		return nil
	}
	return safeHTTPURL(media[0].URL)
}

func safeHTTPURL(value string) *string {
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return nil
	}
	return &value
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}
