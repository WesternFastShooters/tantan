package main

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/jobs"
	"tantan.local/tantan-api/internal/ops"
	"tantan.local/tantan-api/internal/storage"
)

type failedWorkerKeychain struct{}

func (failedWorkerKeychain) Get(context.Context, string) (string, error) {
	return "", errors.New("unavailable")
}

func (failedWorkerKeychain) Set(context.Context, string, string) error {
	return errors.New("unavailable")
}

func (failedWorkerKeychain) Delete(context.Context, string) error {
	return errors.New("unavailable")
}

func TestSyncWorkerRejectsInvalidPayloadWithoutCallingUpstream(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	var jobID string
	if err := store.Write(ctx, func(transaction *sql.Tx) error {
		if _, err := transaction.ExecContext(ctx, "INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES(?,?,?,?,?)", "user_worker", "Worker User", "Asia/Shanghai", now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		job, err := jobs.EnqueueTx(ctx, transaction, jobs.EnqueueRequest{UserID: "user_worker", Kind: "sync", DedupeKey: "invalid-payload", Payload: map[string]any{"scope": "payments"}, Now: now})
		jobID = job.ID
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := runOneSyncJob(ctx, store, nil, func() time.Time { return now }); err != nil {
		t.Fatal(err)
	}
	var state, errorCode string
	if err := store.DB().QueryRowContext(ctx, "SELECT state,error_code FROM jobs WHERE job_id=?", jobID).Scan(&state, &errorCode); err != nil {
		t.Fatal(err)
	}
	if state != "failed" || errorCode != "JOB_PAYLOAD_INVALID" {
		t.Fatalf("state=%s errorCode=%s", state, errorCode)
	}
}

func TestWorkerDoesNotClaimJobsWhileReadinessFailsClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store, err := storage.Open(ctx, storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	var jobID string
	if err := store.Write(ctx, func(transaction *sql.Tx) error {
		if _, err := transaction.ExecContext(ctx, "INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES(?,?,?,?,?)", "user_not_ready", "Not Ready", "Asia/Shanghai", now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		job, err := jobs.EnqueueTx(ctx, transaction, jobs.EnqueueRequest{UserID: "user_not_ready", Kind: "sync", DedupeKey: "not-ready", Payload: map[string]any{"scope": "all"}, Now: now})
		jobID = job.ID
		return err
	}); err != nil {
		t.Fatal(err)
	}
	readiness, err := ops.NewReadiness(ops.ReadinessConfig{DB: store.DB(), Keychain: failedWorkerKeychain{}, Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	application := &application{Store: store}
	application.startWorkers(ctx, readiness, nil, nil, func() time.Time { return now }, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	<-time.After(2 * workerPollInterval)
	cancel()
	application.workers.Wait()
	var state string
	var attempts int
	if err := store.DB().QueryRowContext(context.Background(), "SELECT state,attempts FROM jobs WHERE job_id=?", jobID).Scan(&state, &attempts); err != nil {
		t.Fatal(err)
	}
	if state != "queued" || attempts != 0 {
		t.Fatalf("unready worker state=%s attempts=%d", state, attempts)
	}
}
