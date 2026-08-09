package recommendation

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"tantan.local/tantan-api/internal/storage"
)

var ErrIdempotencyConflict = errors.New("feedback idempotency key was used for a different request")

var feedbackKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)

type FeedbackConfig struct {
	Store *storage.Store
	Now   func() time.Time
}

type FeedbackService struct {
	store *storage.Store
	now   func() time.Time
}

type FeedbackRequest struct {
	UserID         string
	EntryID        string
	Action         string
	TopicID        string
	IdempotencyKey string
}

type FeedbackResult struct {
	Applied bool `json:"applied"`
}

func NewFeedbackService(config FeedbackConfig) (*FeedbackService, error) {
	if config.Store == nil {
		return nil, errors.New("feedback storage is required")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &FeedbackService{store: config.Store, now: now}, nil
}

func (service *FeedbackService) Apply(ctx context.Context, request FeedbackRequest) (FeedbackResult, error) {
	request.UserID = strings.TrimSpace(request.UserID)
	request.EntryID = strings.TrimSpace(request.EntryID)
	request.Action = strings.TrimSpace(request.Action)
	request.TopicID = strings.TrimSpace(request.TopicID)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.UserID == "" || request.EntryID == "" || len(request.IdempotencyKey) < 16 || len(request.IdempotencyKey) > 128 || !feedbackKeyPattern.MatchString(request.IdempotencyKey) {
		return FeedbackResult{}, errors.New("valid feedback identity and idempotency key are required")
	}
	if request.Action != "not_interested" && request.Action != "block_source" && request.Action != "block_topic" && request.Action != "undo" {
		return FeedbackResult{}, errors.New("feedback action is invalid")
	}
	if request.Action == "block_topic" && request.TopicID == "" {
		return FeedbackResult{}, errors.New("block_topic requires topicId")
	}
	if request.Action != "block_topic" && request.Action != "undo" && request.TopicID != "" {
		return FeedbackResult{}, errors.New("topicId is not valid for feedback action")
	}
	now := service.now().UTC()
	err := service.store.Write(ctx, func(transaction *sql.Tx) error {
		replayed, err := feedbackReplay(ctx, transaction, request)
		if err != nil || replayed {
			return err
		}
		var sourceID sql.NullString
		var contentHash string
		if err := transaction.QueryRowContext(ctx, `
SELECT e.feed_id,e.content_hash
FROM entries e JOIN account_entries ae ON ae.entry_id=e.entry_id
WHERE ae.user_id=? AND e.entry_id=?`, request.UserID, request.EntryID).Scan(&sourceID, &contentHash); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("feedback entry was not found")
			}
			return err
		}
		eventTopic := request.TopicID
		eventSource := ""
		targetType := ""
		targetID := ""
		switch request.Action {
		case "not_interested":
			targetType, targetID = "entry", request.EntryID
		case "block_source":
			if !sourceID.Valid || sourceID.String == "" {
				return errors.New("feedback entry has no source")
			}
			eventSource = sourceID.String
			targetType, targetID = "source", sourceID.String
		case "block_topic":
			var exists int
			if err := transaction.QueryRowContext(ctx, `
SELECT COUNT(*) FROM entry_topics
WHERE user_id=? AND entry_id=? AND topic_id=? AND content_hash=?`, request.UserID, request.EntryID, request.TopicID, contentHash).Scan(&exists); err != nil || exists != 1 {
				return errors.New("feedback topic is not assigned to the current entry")
			}
			targetType, targetID = "topic", request.TopicID
		case "undo":
			var priorType string
			var priorTopic sql.NullString
			var priorSource sql.NullString
			err := transaction.QueryRowContext(ctx, `
SELECT event_type,topic_id,source_id FROM recommendation_events
WHERE user_id=? AND entry_id=? AND event_type<>'undo'
ORDER BY created_at DESC,rowid DESC LIMIT 1`, request.UserID, request.EntryID).Scan(&priorType, &priorTopic, &priorSource)
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("feedback action has nothing to undo")
			}
			if err != nil {
				return err
			}
			switch priorType {
			case "not_interested":
				targetType, targetID = "entry", request.EntryID
			case "block_source":
				if !priorSource.Valid {
					return errors.New("stored source feedback is invalid")
				}
				eventSource = priorSource.String
				targetType, targetID = "source", priorSource.String
			case "block_topic":
				if !priorTopic.Valid {
					return errors.New("stored topic feedback is invalid")
				}
				eventTopic = priorTopic.String
				targetType, targetID = "topic", priorTopic.String
			}
		}
		if request.Action == "undo" {
			if _, err := transaction.ExecContext(ctx, "DELETE FROM recommendation_blocks WHERE user_id=? AND target_type=? AND target_id=?", request.UserID, targetType, targetID); err != nil {
				return err
			}
		} else {
			if _, err := transaction.ExecContext(ctx, `
INSERT INTO recommendation_blocks(user_id,target_type,target_id,strength,created_at)
VALUES(?,?,?,1,?)
ON CONFLICT(user_id,target_type,target_id) DO UPDATE SET strength=1,created_at=excluded.created_at`, request.UserID, targetType, targetID, now.Format(time.RFC3339Nano)); err != nil {
				return err
			}
			if err := removeBlockedQueueItems(ctx, transaction, request.UserID, targetType, targetID, now); err != nil {
				return err
			}
		}
		eventID, err := newFeedbackEventID()
		if err != nil {
			return err
		}
		var topic any
		if eventTopic != "" {
			topic = eventTopic
		}
		var source any
		if eventSource != "" {
			source = eventSource
		}
		_, err = transaction.ExecContext(ctx, `
INSERT INTO recommendation_events(event_id,user_id,entry_id,event_type,topic_id,source_id,idempotency_key,created_at)
VALUES(?,?,?,?,?,?,?,?)`, eventID, request.UserID, request.EntryID, request.Action, topic, source, request.IdempotencyKey, now.Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		return FeedbackResult{}, err
	}
	return FeedbackResult{Applied: true}, nil
}

func feedbackReplay(ctx context.Context, transaction *sql.Tx, request FeedbackRequest) (bool, error) {
	var entryID string
	var action string
	var topic sql.NullString
	err := transaction.QueryRowContext(ctx, `
SELECT entry_id,event_type,topic_id FROM recommendation_events
WHERE user_id=? AND idempotency_key=?`, request.UserID, request.IdempotencyKey).Scan(&entryID, &action, &topic)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if entryID != request.EntryID || action != request.Action || (request.TopicID != "" && (!topic.Valid || topic.String != request.TopicID)) {
		return false, ErrIdempotencyConflict
	}
	return true, nil
}

func removeBlockedQueueItems(ctx context.Context, transaction *sql.Tx, userID, targetType, targetID string, now time.Time) error {
	statement := `
UPDATE daily_queue_items SET state='removed',updated_at=?
WHERE queue_id IN (SELECT queue_id FROM daily_queues WHERE user_id=? AND status='ready') AND state<>'removed'`
	arguments := []any{now.Format(time.RFC3339Nano), userID}
	switch targetType {
	case "entry":
		statement += " AND entry_id=?"
		arguments = append(arguments, targetID)
	case "source":
		statement += " AND entry_id IN (SELECT entry_id FROM entries WHERE feed_id=?)"
		arguments = append(arguments, targetID)
	case "topic":
		statement += ` AND entry_id IN (
  SELECT et.entry_id FROM entry_topics et JOIN entries e ON e.entry_id=et.entry_id
  WHERE et.user_id=? AND et.topic_id=? AND et.content_hash=e.content_hash
)`
		arguments = append(arguments, userID, targetID)
	default:
		return fmt.Errorf("unsupported feedback target %s", targetType)
	}
	_, err := transaction.ExecContext(ctx, statement, arguments...)
	return err
}

func newFeedbackEventID() (string, error) {
	buffer := make([]byte, 18)
	if _, err := rand.Read(buffer); err != nil {
		return "", errors.New("create feedback event ID failed")
	}
	return "event_" + base64.RawURLEncoding.EncodeToString(buffer), nil
}
