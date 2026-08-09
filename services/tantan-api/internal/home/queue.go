package home

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"tantan.local/tantan-api/internal/recommendation"
)

type queueRecord struct {
	ID          string
	Generation  string
	UserID      string
	LocalDate   string
	FilterKey   string
	Timezone    string
	Version     int
	GeneratedAt time.Time
}

type queueWatermark struct {
	EntryRowID   int64
	AccountRowID int64
}

func (service *Service) ensureQueue(ctx context.Context, request PlanRequest) (queueRecord, error) {
	if strings.TrimSpace(request.Timezone) == "" {
		request.Timezone = "Asia/Shanghai"
	}
	if strings.TrimSpace(request.FilterKey) == "" {
		request.FilterKey = defaultFilterKey
	}
	_, localDate, _, err := calendarWindow(service.now().UTC(), request.Timezone, request.Spec)
	if err != nil {
		return queueRecord{}, err
	}
	ready, found, err := service.queryReady(ctx, request.UserID, localDate, request.FilterKey)
	if err != nil {
		return queueRecord{}, err
	}
	if found {
		if err := service.appendIfChanged(ctx, ready, request.Spec); err != nil {
			return queueRecord{}, err
		}
		return service.queryReadyRequired(ctx, request.UserID, localDate, request.FilterKey)
	}
	plan, err := service.Plan(ctx, request)
	if err != nil {
		return queueRecord{}, err
	}
	var result queueRecord
	err = service.store.Write(ctx, func(transaction *sql.Tx) error {
		current, found, err := queryReadyTx(ctx, transaction, plan.UserID, plan.LocalDate, plan.FilterKey)
		if err != nil {
			return err
		}
		if found {
			result = current
			return nil
		}
		state, err := service.persistPlanTx(ctx, transaction, plan, false)
		if err != nil {
			return err
		}
		result = queueRecord{ID: state.ID, Generation: state.Generation, UserID: plan.UserID, LocalDate: plan.LocalDate, FilterKey: plan.FilterKey, Timezone: plan.Timezone, Version: state.Version, GeneratedAt: plan.GeneratedAt}
		return nil
	})
	if err != nil {
		return queueRecord{}, err
	}
	return result, nil
}

func (service *Service) appendIfChanged(ctx context.Context, queue queueRecord, spec *recommendation.FilterSpecV1) error {
	service.appendMutex.Lock()
	defer service.appendMutex.Unlock()
	current, err := service.queueWatermark(ctx, queue.UserID)
	if err != nil {
		return err
	}
	if previous, found := service.appendWatermarks[queue.ID]; found && previous == current {
		return nil
	}
	if err := service.appendNew(ctx, queue, spec); err != nil {
		return err
	}
	service.appendWatermarks[queue.ID] = current
	return nil
}

func (service *Service) queueWatermark(ctx context.Context, userID string) (queueWatermark, error) {
	var result queueWatermark
	if err := service.store.DB().QueryRowContext(ctx, "SELECT COALESCE(MAX(rowid),0) FROM entries").Scan(&result.EntryRowID); err != nil {
		return queueWatermark{}, fmt.Errorf("read entry watermark: %w", err)
	}
	if err := service.store.DB().QueryRowContext(ctx, "SELECT COALESCE(MAX(rowid),0) FROM account_entries WHERE user_id=?", userID).Scan(&result.AccountRowID); err != nil {
		return queueWatermark{}, fmt.Errorf("read account-entry watermark: %w", err)
	}
	return result, nil
}

func (service *Service) ReplaceTx(ctx context.Context, transaction *sql.Tx, plan QueuePlan) (QueueState, error) {
	return service.persistPlanTx(ctx, transaction, plan, true)
}

func (service *Service) EnsurePlanTx(ctx context.Context, transaction *sql.Tx, plan QueuePlan) (QueueState, error) {
	if transaction == nil {
		return QueueState{}, errors.New("queue transaction is required")
	}
	current, found, err := queryReadyTx(ctx, transaction, plan.UserID, plan.LocalDate, plan.FilterKey)
	if err != nil {
		return QueueState{}, err
	}
	if found {
		return queueStateTx(ctx, transaction, current, "recommend")
	}
	return service.persistPlanTx(ctx, transaction, plan, false)
}

func (service *Service) persistPlanTx(ctx context.Context, transaction *sql.Tx, plan QueuePlan, replace bool) (QueueState, error) {
	if transaction == nil || !validHomeID(plan.ID) || !validHomeID(plan.UserID) || !validHomeID(plan.FilterKey) || len(plan.LocalDate) != 10 || plan.GeneratedAt.IsZero() || len(plan.Items) > initialQueueSize {
		return QueueState{}, errors.New("valid queue plan and transaction are required")
	}
	if _, _, _, err := calendarWindow(plan.GeneratedAt, plan.Timezone, nil); err != nil {
		return QueueState{}, err
	}
	var version int
	if err := transaction.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0)+1 FROM daily_queues WHERE user_id=? AND local_date=? AND filter_key=?", plan.UserID, plan.LocalDate, plan.FilterKey).Scan(&version); err != nil {
		return QueueState{}, fmt.Errorf("choose queue version: %w", err)
	}
	timestamp := plan.GeneratedAt.UTC().Format(time.RFC3339Nano)
	generation := fmt.Sprintf("%s-v%d", plan.ID, version)
	var filterID any
	if plan.FilterKey != defaultFilterKey {
		filterID = plan.FilterKey
	}
	if replace {
		if _, err := transaction.ExecContext(ctx, `
UPDATE daily_queues SET status='superseded',updated_at=?
WHERE user_id=? AND local_date=? AND filter_key=? AND status='ready'`, timestamp, plan.UserID, plan.LocalDate, plan.FilterKey); err != nil {
			return QueueState{}, fmt.Errorf("supersede queue: %w", err)
		}
	}
	if _, err := transaction.ExecContext(ctx, `
INSERT INTO daily_queues(queue_id,user_id,local_date,filter_key,timezone,target_size,hard_limit,status,version,generated_at,created_at,updated_at,generation,topic_id,filter_id)
VALUES(?,?,?,?,?,50,60,'building',?,?,?,?,?,'recommend',?)`, plan.ID, plan.UserID, plan.LocalDate, plan.FilterKey, plan.Timezone, version, timestamp, timestamp, timestamp, generation, filterID); err != nil {
		return QueueState{}, fmt.Errorf("create queue: %w", err)
	}
	for index, item := range plan.Items {
		scoreJSON, err := json.Marshal(item.Score)
		if err != nil {
			return QueueState{}, errors.New("encode queue score")
		}
		if _, err := transaction.ExecContext(ctx, `
INSERT INTO daily_queue_items(queue_id,entry_id,rank,score,score_json,state,added_at,updated_at)
VALUES(?,?,?,?,?,'unread',?,?)`, plan.ID, item.EntryID, index+1, item.Score.Total, string(scoreJSON), timestamp, timestamp); err != nil {
			return QueueState{}, fmt.Errorf("insert queue item: %w", err)
		}
	}
	if _, err := transaction.ExecContext(ctx, "UPDATE daily_queues SET status='ready',generated_at=?,updated_at=? WHERE queue_id=? AND status='building'", timestamp, timestamp, plan.ID); err != nil {
		return QueueState{}, fmt.Errorf("publish queue: %w", err)
	}
	record := queueRecord{ID: plan.ID, Generation: generation, UserID: plan.UserID, LocalDate: plan.LocalDate, FilterKey: plan.FilterKey, Timezone: plan.Timezone, Version: version, GeneratedAt: plan.GeneratedAt.UTC()}
	return queueStateTx(ctx, transaction, record, "recommend")
}

func (service *Service) appendNew(ctx context.Context, queue queueRecord, spec *recommendation.FilterSpecV1) error {
	var count int
	if err := service.store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM daily_queue_items WHERE queue_id=?", queue.ID).Scan(&count); err != nil {
		return fmt.Errorf("count queue items: %w", err)
	}
	if count >= queueHardLimit {
		return nil
	}
	now := service.now().UTC()
	candidates, err := service.loadCandidates(ctx, candidateRequest{UserID: queue.UserID, Timezone: queue.Timezone, Spec: spec, Now: now, CreatedAfter: &queue.GeneratedAt, ExcludeQueue: queue.ID})
	if err != nil {
		return err
	}
	previousSources, err := service.lastQueueSources(ctx, queue.ID)
	if err != nil {
		return err
	}
	ranked := recommendation.RankAfterSources(now, candidates, queueHardLimit-count, previousSources)
	if len(ranked) == 0 {
		return nil
	}
	return service.store.Write(ctx, func(transaction *sql.Tx) error {
		var currentCount int
		var maxRank int
		if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(MAX(rank),0) FROM daily_queue_items WHERE queue_id=?", queue.ID).Scan(&currentCount, &maxRank); err != nil {
			return err
		}
		capacity := queueHardLimit - currentCount
		if capacity <= 0 {
			return nil
		}
		if len(ranked) > capacity {
			ranked = ranked[:capacity]
		}
		timestamp := now.Format(time.RFC3339Nano)
		for index, item := range ranked {
			scoreJSON, err := json.Marshal(item.Score)
			if err != nil {
				return err
			}
			if _, err := transaction.ExecContext(ctx, `
INSERT OR IGNORE INTO daily_queue_items(queue_id,entry_id,rank,score,score_json,state,added_at,updated_at)
VALUES(?,?,?,?,?,'unread',?,?)`, queue.ID, item.EntryID, maxRank+index+1, item.Score.Total, string(scoreJSON), timestamp, timestamp); err != nil {
				return err
			}
		}
		_, err := transaction.ExecContext(ctx, "UPDATE daily_queues SET updated_at=? WHERE queue_id=? AND status='ready'", timestamp, queue.ID)
		return err
	})
}

func (service *Service) lastQueueSources(ctx context.Context, queueID string) ([]string, error) {
	rows, err := service.store.DB().QueryContext(ctx, `
SELECT e.feed_id FROM daily_queue_items qi
JOIN entries e ON e.entry_id=qi.entry_id
WHERE qi.queue_id=? ORDER BY qi.rank DESC LIMIT 2`, queueID)
	if err != nil {
		return nil, fmt.Errorf("read queue source tail: %w", err)
	}
	defer rows.Close()
	var reversed []string
	for rows.Next() {
		var sourceID string
		if err := rows.Scan(&sourceID); err != nil {
			return nil, fmt.Errorf("scan queue source tail: %w", err)
		}
		reversed = append(reversed, sourceID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queue source tail: %w", err)
	}
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	return reversed, nil
}

func (service *Service) queryReady(ctx context.Context, userID, localDate, filterKey string) (queueRecord, bool, error) {
	var record queueRecord
	var generated string
	err := service.store.DB().QueryRowContext(ctx, `
SELECT queue_id,generation,user_id,local_date,filter_key,timezone,version,generated_at
FROM daily_queues WHERE user_id=? AND local_date=? AND filter_key=? AND status='ready'`, userID, localDate, filterKey).Scan(&record.ID, &record.Generation, &record.UserID, &record.LocalDate, &record.FilterKey, &record.Timezone, &record.Version, &generated)
	if errors.Is(err, sql.ErrNoRows) {
		return queueRecord{}, false, nil
	}
	if err != nil {
		return queueRecord{}, false, fmt.Errorf("read ready queue: %w", err)
	}
	if !validHomeID(record.Generation) {
		return queueRecord{}, false, errors.New("ready queue has invalid generation")
	}
	record.GeneratedAt, err = time.Parse(time.RFC3339Nano, generated)
	if err != nil {
		return queueRecord{}, false, errors.New("ready queue has invalid generated time")
	}
	return record, true, nil
}

func (service *Service) queryReadyRequired(ctx context.Context, userID, localDate, filterKey string) (queueRecord, error) {
	record, found, err := service.queryReady(ctx, userID, localDate, filterKey)
	if err != nil {
		return queueRecord{}, err
	}
	if !found {
		return queueRecord{}, errors.New("ready queue disappeared")
	}
	return record, nil
}

func queryReadyTx(ctx context.Context, transaction *sql.Tx, userID, localDate, filterKey string) (queueRecord, bool, error) {
	var record queueRecord
	var generated string
	err := transaction.QueryRowContext(ctx, `
SELECT queue_id,generation,user_id,local_date,filter_key,timezone,version,generated_at
FROM daily_queues WHERE user_id=? AND local_date=? AND filter_key=? AND status='ready'`, userID, localDate, filterKey).Scan(&record.ID, &record.Generation, &record.UserID, &record.LocalDate, &record.FilterKey, &record.Timezone, &record.Version, &generated)
	if errors.Is(err, sql.ErrNoRows) {
		return queueRecord{}, false, nil
	}
	if err != nil {
		return queueRecord{}, false, err
	}
	if !validHomeID(record.Generation) {
		return queueRecord{}, false, errors.New("ready queue has invalid generation")
	}
	record.GeneratedAt, err = time.Parse(time.RFC3339Nano, generated)
	if err != nil {
		return queueRecord{}, false, err
	}
	return record, true, nil
}

func (service *Service) reconcileReads(ctx context.Context, userID, queueID string) error {
	now := service.now().UTC().Format(time.RFC3339Nano)
	return service.store.Write(ctx, func(transaction *sql.Tx) error {
		_, err := transaction.ExecContext(ctx, `
UPDATE daily_queue_items SET state='read',updated_at=?
WHERE queue_id=? AND state='unread' AND entry_id IN (
  SELECT entry_id FROM account_entries WHERE user_id=? AND read_at IS NOT NULL
)`, now, queueID, userID)
		return err
	})
}
