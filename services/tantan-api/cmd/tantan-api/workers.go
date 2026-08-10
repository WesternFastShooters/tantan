package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"tantan.local/tantan-api/internal/enrichment"
	"tantan.local/tantan-api/internal/jobs"
	"tantan.local/tantan-api/internal/ops"
	"tantan.local/tantan-api/internal/storage"
	syncer "tantan.local/tantan-api/internal/sync"
	"tantan.local/tantan-api/internal/topic"
)

const workerPollInterval = 500 * time.Millisecond

func (application *application) startWorkers(ctx context.Context, readiness *ops.Readiness, enrichmentService *enrichment.Service, syncService *syncer.Service, topics *topic.Service, now func() time.Time, logger *slog.Logger) {
	application.workers.Add(1)
	go func() {
		defer application.workers.Done()
		ticker := time.NewTicker(workerPollInterval)
		defer ticker.Stop()
		ready := false
		nextReadiness := time.Time{}
		for {
			current := time.Now().UTC()
			if !current.Before(nextReadiness) {
				ready = readiness.Check(ctx).Ready
				nextReadiness = current.Add(30 * time.Second)
			}
			if ready {
				if _, err := enrichmentService.RunOne(ctx); err != nil && !errors.Is(err, context.Canceled) {
					logger.ErrorContext(ctx, "worker_failed", slog.String("module", "enrichment"), slog.String("errorCode", "LOCAL_STORAGE_ERROR"))
					nextReadiness = time.Time{}
				}
				if err := runOneSyncJob(ctx, application.Store, syncService, topics, now); err != nil && !errors.Is(err, context.Canceled) {
					logger.ErrorContext(ctx, "worker_failed", slog.String("module", "sync"), slog.String("errorCode", "LOCAL_STORAGE_ERROR"))
					nextReadiness = time.Time{}
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func runOneSyncJob(ctx context.Context, store *storage.Store, service *syncer.Service, topics *topic.Service, now func() time.Time) error {
	current := now().UTC()
	job, found, err := jobs.ClaimNext(ctx, store, "sync", current, 90*time.Second, 3)
	if err != nil || !found {
		return err
	}
	var payload struct {
		Scope string `json:"scope"`
	}
	decoderErr := json.Unmarshal([]byte(job.PayloadJSON), &payload)
	if decoderErr != nil || !validSyncScope(payload.Scope) {
		return finishInvalidSyncJob(ctx, store, job.ID, current)
	}
	var account syncer.Account
	var avatar sql.NullString
	err = store.DB().QueryRowContext(ctx, "SELECT user_id,name,avatar,timezone FROM accounts WHERE user_id=?", job.UserID).Scan(&account.ID, &account.Name, &avatar, &account.Timezone)
	if err != nil {
		return finishInvalidSyncJob(ctx, store, job.ID, current)
	}
	if avatar.Valid {
		account.Avatar = &avatar.String
	}
	_, runErr := service.Run(ctx, account, syncer.RunOptions{Mode: syncer.ModeAuto})
	if runErr == nil {
		if err := topics.RefreshGenerated(ctx, account.ID); err != nil {
			return err
		}
		return jobs.Succeed(ctx, store, job.ID, now().UTC())
	}
	_, retryErr := jobs.Retry(ctx, store, job, payload, "FOLO_UNAVAILABLE", now().UTC(), 3)
	return retryErr
}

func finishInvalidSyncJob(ctx context.Context, store *storage.Store, jobID string, now time.Time) error {
	return store.Write(ctx, func(transaction *sql.Tx) error {
		return jobs.FinishTx(ctx, transaction, jobID, "failed", "JOB_PAYLOAD_INVALID", now)
	})
}
