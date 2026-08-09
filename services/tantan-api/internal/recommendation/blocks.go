package recommendation

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type SourceBlock struct {
	SourceID  string `json:"sourceId"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

type RestoreSourceBlockRequest struct {
	UserID         string
	SourceID       string
	IdempotencyKey string
}

func (service *FeedbackService) ListSourceBlocks(ctx context.Context, userID string) ([]SourceBlock, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, errors.New("valid user is required")
	}
	rows, err := service.store.DB().QueryContext(ctx, `
SELECT b.target_id,COALESCE(NULLIF(f.title,''),b.target_id),b.created_at
FROM recommendation_blocks b
LEFT JOIN feeds f ON f.feed_id=b.target_id
WHERE b.user_id=? AND b.target_type='source'
ORDER BY b.created_at DESC,b.target_id ASC
LIMIT 500`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	blocks := make([]SourceBlock, 0)
	for rows.Next() {
		var block SourceBlock
		if err := rows.Scan(&block.SourceID, &block.Name, &block.CreatedAt); err != nil {
			return nil, err
		}
		if _, err := time.Parse(time.RFC3339Nano, block.CreatedAt); err != nil {
			return nil, errors.New("stored Source block time is invalid")
		}
		blocks = append(blocks, block)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return blocks, nil
}

func (service *FeedbackService) RestoreSourceBlock(ctx context.Context, request RestoreSourceBlockRequest) error {
	request.UserID = strings.TrimSpace(request.UserID)
	request.SourceID = strings.TrimSpace(request.SourceID)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.UserID == "" || request.SourceID == "" || len(request.SourceID) > 128 || !feedbackKeyPattern.MatchString(request.SourceID) || len(request.IdempotencyKey) < 16 || len(request.IdempotencyKey) > 128 || !feedbackKeyPattern.MatchString(request.IdempotencyKey) {
		return errors.New("valid Source block identity and idempotency key are required")
	}

	return service.store.Write(ctx, func(transaction *sql.Tx) error {
		var replaySource sql.NullString
		var replayType string
		err := transaction.QueryRowContext(ctx, `
SELECT source_id,event_type FROM recommendation_events
WHERE user_id=? AND idempotency_key=?`, request.UserID, request.IdempotencyKey).Scan(&replaySource, &replayType)
		if err == nil {
			if replayType == "undo" && replaySource.Valid && replaySource.String == request.SourceID {
				return nil
			}
			return ErrIdempotencyConflict
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		var entryID string
		err = transaction.QueryRowContext(ctx, `
SELECT event.entry_id
FROM recommendation_blocks block
JOIN recommendation_events event
  ON event.user_id=block.user_id
 AND event.source_id=block.target_id
 AND event.event_type='block_source'
WHERE block.user_id=? AND block.target_type='source' AND block.target_id=?
ORDER BY event.created_at DESC,event.rowid DESC
LIMIT 1`, request.UserID, request.SourceID).Scan(&entryID)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("Source block was not found")
		}
		if err != nil {
			return err
		}
		result, err := transaction.ExecContext(ctx, `
DELETE FROM recommendation_blocks
WHERE user_id=? AND target_type='source' AND target_id=?`, request.UserID, request.SourceID)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return errors.New("Source block was not restored")
		}

		eventID, err := newFeedbackEventID()
		if err != nil {
			return err
		}
		_, err = transaction.ExecContext(ctx, `
INSERT INTO recommendation_events(event_id,user_id,entry_id,event_type,topic_id,source_id,idempotency_key,created_at)
VALUES(?,?,?,'undo',NULL,?,?,?)`, eventID, request.UserID, entryID, request.SourceID, request.IdempotencyKey, service.now().UTC().Format(time.RFC3339Nano))
		return err
	})
}
