package jobs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"tantan.local/tantan-api/internal/storage"
)

type Job struct {
	ID          string
	UserID      string
	Kind        string
	PayloadJSON string
	Attempts    int
}

type EnqueueRequest struct {
	UserID    string
	Kind      string
	DedupeKey string
	Payload   any
	Now       time.Time
}

func EnqueueTx(ctx context.Context, transaction *sql.Tx, request EnqueueRequest) (Job, error) {
	if transaction == nil || strings.TrimSpace(request.UserID) == "" || strings.TrimSpace(request.Kind) == "" || strings.TrimSpace(request.DedupeKey) == "" || request.Now.IsZero() {
		return Job{}, errors.New("valid job enqueue request is required")
	}
	payload, err := json.Marshal(request.Payload)
	if err != nil {
		return Job{}, errors.New("encode job payload")
	}
	jobID, err := newJobID()
	if err != nil {
		return Job{}, err
	}
	timestamp := request.Now.UTC().Format(time.RFC3339Nano)
	if _, err := transaction.ExecContext(ctx, `
INSERT OR IGNORE INTO jobs(job_id,user_id,kind,dedupe_key,state,payload_json,attempts,next_run_at,created_at,updated_at)
VALUES(?,?,?,?, 'queued',?,0,?,?,?)`, jobID, request.UserID, request.Kind, request.DedupeKey, string(payload), timestamp, timestamp, timestamp); err != nil {
		return Job{}, fmt.Errorf("enqueue job: %w", err)
	}
	var job Job
	if err := transaction.QueryRowContext(ctx, `
SELECT job_id,user_id,kind,payload_json,attempts
FROM jobs
WHERE kind=? AND dedupe_key=? AND state IN ('queued','running')
ORDER BY created_at,job_id LIMIT 1`, request.Kind, request.DedupeKey).Scan(&job.ID, &job.UserID, &job.Kind, &job.PayloadJSON, &job.Attempts); err != nil {
		return Job{}, fmt.Errorf("read enqueued job: %w", err)
	}
	return job, nil
}

func EnqueueContentRetryTx(ctx context.Context, transaction *sql.Tx, userID string, entryIDs []string, now time.Time) error {
	if transaction == nil || userID == "" || len(entryIDs) < 1 || len(entryIDs) > 50 {
		return errors.New("content retry requires a user and 1 to 50 entry IDs")
	}
	ids := append([]string(nil), entryIDs...)
	sort.Strings(ids)
	for index, id := range ids {
		if strings.TrimSpace(id) == "" || (index > 0 && id == ids[index-1]) {
			return errors.New("content retry entry IDs must be non-empty and unique")
		}
	}
	payload, err := json.Marshal(map[string]any{"entryIds": ids})
	if err != nil {
		return fmt.Errorf("encode content retry job: %w", err)
	}
	digest := sha256.Sum256(append([]byte(userID+"\x00"), payload...))
	dedupeKey := "content:" + hex.EncodeToString(digest[:])
	jobID, err := newJobID()
	if err != nil {
		return err
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)
	_, err = transaction.ExecContext(ctx, `
INSERT OR IGNORE INTO jobs(job_id,user_id,kind,dedupe_key,state,payload_json,attempts,next_run_at,created_at,updated_at)
VALUES(?,?, 'content',?, 'queued',?,0,?,?,?)`, jobID, userID, dedupeKey, string(payload), timestamp, timestamp, timestamp)
	if err != nil {
		return fmt.Errorf("enqueue content retry job: %w", err)
	}
	return nil
}

func newJobID() (string, error) {
	buffer := make([]byte, 18)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("create job ID: %w", err)
	}
	return "job_" + base64.RawURLEncoding.EncodeToString(buffer), nil
}

func ClaimNext(ctx context.Context, store *storage.Store, kind string, now time.Time, lease time.Duration, maxAttempts int) (Job, bool, error) {
	if store == nil || strings.TrimSpace(kind) == "" || lease <= 0 || maxAttempts < 1 {
		return Job{}, false, errors.New("job claim requires storage, kind, lease, and attempts")
	}
	var claimed Job
	found := false
	timestamp := now.UTC().Format(time.RFC3339Nano)
	leaseUntil := now.Add(lease).UTC().Format(time.RFC3339Nano)
	err := store.Write(ctx, func(transaction *sql.Tx) error {
		if _, err := transaction.ExecContext(ctx, `
UPDATE jobs
SET state='failed',lease_until=NULL,error_code='JOB_ATTEMPTS_EXHAUSTED',updated_at=?,finished_at=?
WHERE kind=? AND state='running' AND lease_until IS NOT NULL AND lease_until<=? AND attempts>=?`, timestamp, timestamp, kind, timestamp, maxAttempts); err != nil {
			return fmt.Errorf("expire exhausted jobs: %w", err)
		}
		var userID sql.NullString
		err := transaction.QueryRowContext(ctx, `
SELECT job_id,user_id,kind,payload_json,attempts
FROM jobs
WHERE kind=? AND attempts<? AND (
  (state='queued' AND next_run_at<=?) OR
  (state='running' AND lease_until IS NOT NULL AND lease_until<=?)
)
ORDER BY next_run_at,created_at,job_id
LIMIT 1`, kind, maxAttempts, timestamp, timestamp).Scan(&claimed.ID, &userID, &claimed.Kind, &claimed.PayloadJSON, &claimed.Attempts)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("select job: %w", err)
		}
		if userID.Valid {
			claimed.UserID = userID.String
		}
		result, err := transaction.ExecContext(ctx, `
UPDATE jobs
SET state='running',attempts=attempts+1,lease_until=?,error_code=NULL,updated_at=?,finished_at=NULL
WHERE job_id=? AND (
  (state='queued' AND next_run_at<=? AND attempts<?) OR
  (state='running' AND lease_until IS NOT NULL AND lease_until<=? AND attempts<?)
)`, leaseUntil, timestamp, claimed.ID, timestamp, maxAttempts, timestamp, maxAttempts)
		if err != nil {
			return fmt.Errorf("claim job: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect claimed job: %w", err)
		}
		if affected != 1 {
			return errors.New("job claim lost")
		}
		claimed.Attempts++
		found = true
		return nil
	})
	if err != nil {
		return Job{}, false, err
	}
	return claimed, found, nil
}

func Succeed(ctx context.Context, store *storage.Store, jobID string, now time.Time) error {
	if store == nil || jobID == "" {
		return errors.New("job completion requires storage and ID")
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)
	return store.Write(ctx, func(transaction *sql.Tx) error {
		result, err := transaction.ExecContext(ctx, `
UPDATE jobs
SET state='succeeded',lease_until=NULL,error_code=NULL,updated_at=?,finished_at=?
WHERE job_id=? AND state='running'`, timestamp, timestamp, jobID)
		if err != nil {
			return fmt.Errorf("complete job: %w", err)
		}
		return requireOneJob(result)
	})
}

func FinishTx(ctx context.Context, transaction *sql.Tx, jobID, state, errorCode string, now time.Time) error {
	if transaction == nil || jobID == "" || (state != "succeeded" && state != "failed" && state != "cancelled") || now.IsZero() {
		return errors.New("valid terminal job transition is required")
	}
	var code any
	if errorCode != "" {
		code = errorCode
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)
	result, err := transaction.ExecContext(ctx, `
UPDATE jobs
SET state=?,lease_until=NULL,error_code=?,updated_at=?,finished_at=?
WHERE job_id=? AND state='running'`, state, code, timestamp, timestamp, jobID)
	if err != nil {
		return fmt.Errorf("finish job: %w", err)
	}
	return requireOneJob(result)
}

func Retry(ctx context.Context, store *storage.Store, job Job, payload any, errorCode string, now time.Time, maxAttempts int) (bool, error) {
	if store == nil || job.ID == "" || strings.TrimSpace(errorCode) == "" || maxAttempts < 1 {
		return false, errors.New("job retry requires storage, job, error code, and attempts")
	}
	var terminal bool
	err := store.Write(ctx, func(transaction *sql.Tx) error {
		var err error
		terminal, err = RetryTx(ctx, transaction, job, payload, errorCode, now, maxAttempts)
		return err
	})
	return terminal, err
}

func RetryTx(ctx context.Context, transaction *sql.Tx, job Job, payload any, errorCode string, now time.Time, maxAttempts int) (bool, error) {
	if transaction == nil || job.ID == "" || strings.TrimSpace(errorCode) == "" || now.IsZero() || maxAttempts < 1 {
		return false, errors.New("valid job retry transition is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return false, errors.New("encode retry job payload")
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)
	terminal := job.Attempts >= maxAttempts
	state := "queued"
	nextRun := now.Add(retryDelay(job.Attempts)).UTC().Format(time.RFC3339Nano)
	var finishedAt any
	if terminal {
		state = "failed"
		nextRun = timestamp
		finishedAt = timestamp
	}
	result, err := transaction.ExecContext(ctx, `
UPDATE jobs
SET state=?,payload_json=?,next_run_at=?,lease_until=NULL,error_code=?,updated_at=?,finished_at=?
WHERE job_id=? AND state='running'`, state, string(encoded), nextRun, errorCode, timestamp, finishedAt, job.ID)
	if err != nil {
		return false, fmt.Errorf("retry job: %w", err)
	}
	return terminal, requireOneJob(result)
}

func requireOneJob(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect job transition: %w", err)
	}
	if affected != 1 {
		return errors.New("job is not running")
	}
	return nil
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Duration(1<<(attempt-1)) * time.Minute
	if delay > 15*time.Minute {
		return 15 * time.Minute
	}
	return delay
}
