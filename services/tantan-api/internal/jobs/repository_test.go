package jobs_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/jobs"
	"tantan.local/tantan-api/internal/storage"
)

func TestExpiredJobLeaseIsReclaimedAndAttemptsAreBounded(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	if err := store.Write(ctx, func(transaction *sql.Tx) error {
		if _, err := transaction.ExecContext(ctx, "INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES(?,?,?,?,?)", "user_1", "Test User", "Asia/Shanghai", now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		return jobs.EnqueueContentRetryTx(ctx, transaction, "user_1", []string{"entry_1"}, now)
	}); err != nil {
		t.Fatalf("enqueue content job: %v", err)
	}

	first, found, err := jobs.ClaimNext(ctx, store, "content", now, 2*time.Minute, 2)
	if err != nil || !found || first.Attempts != 1 {
		t.Fatalf("first claim=%#v found=%t err=%v", first, found, err)
	}
	if _, found, err := jobs.ClaimNext(ctx, store, "content", now.Add(time.Minute), 2*time.Minute, 2); err != nil || found {
		t.Fatalf("active lease found=%t err=%v", found, err)
	}
	second, found, err := jobs.ClaimNext(ctx, store, "content", now.Add(2*time.Minute), 2*time.Minute, 2)
	if err != nil || !found || second.ID != first.ID || second.Attempts != 2 {
		t.Fatalf("reclaimed job=%#v found=%t err=%v", second, found, err)
	}
	if _, found, err := jobs.ClaimNext(ctx, store, "content", now.Add(4*time.Minute), 2*time.Minute, 2); err != nil || found {
		t.Fatalf("exhausted claim found=%t err=%v", found, err)
	}
	var state string
	var attempts int
	if err := store.DB().QueryRowContext(ctx, "SELECT state,attempts FROM jobs WHERE job_id=?", first.ID).Scan(&state, &attempts); err != nil {
		t.Fatalf("read terminal job: %v", err)
	}
	if state != "failed" || attempts != 2 {
		t.Fatalf("state=%s attempts=%d", state, attempts)
	}
}
