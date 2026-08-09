package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"tantan.local/tantan-api/internal/jobs"
)

const (
	contentJobLease       = 2 * time.Minute
	contentJobMaxAttempts = 3
)

type contentJobPayload struct {
	EntryIDs []string `json:"entryIds"`
}

func (service *Service) RunOneContentJob(ctx context.Context) (bool, error) {
	now := service.now().UTC()
	job, found, err := jobs.ClaimNext(ctx, service.store, "content", now, contentJobLease, contentJobMaxAttempts)
	if err != nil || !found {
		return found, err
	}
	payload, err := decodeContentJobPayload(job.PayloadJSON)
	if err != nil || job.UserID == "" {
		_, transitionErr := jobs.Retry(ctx, service.store, job, contentJobPayload{}, "JOB_PAYLOAD_INVALID", now, 1)
		if transitionErr != nil {
			return true, transitionErr
		}
		return true, nil
	}
	stream, err := service.source.StreamContents(ctx, job.UserID, payload.EntryIDs)
	if err != nil {
		return true, service.requeueContentJob(ctx, job, payload.EntryIDs, "FOLO_UNAVAILABLE", now)
	}
	defer stream.Close()
	contents, missing, _, err := ParseContentStream(stream, payload.EntryIDs)
	if err != nil {
		return true, service.requeueContentJob(ctx, job, payload.EntryIDs, "FOLO_UNAVAILABLE", now)
	}
	result := contentBatch{contents: contents, missing: missing}
	entryIDs := make([]string, 0, len(result.contents))
	for entryID := range result.contents {
		entryIDs = append(entryIDs, entryID)
	}
	sort.Strings(entryIDs)
	for _, entryID := range entryIDs {
		content := result.contents[entryID]
		if err := service.store.Write(ctx, func(transaction *sql.Tx) error {
			if err := service.repository.UpdateEntryContentTx(ctx, transaction, entryID, content, now); err != nil {
				return err
			}
			return service.indexer.RefreshTx(ctx, transaction, job.UserID, []string{entryID})
		}); err != nil {
			return true, service.requeueContentJob(ctx, job, payload.EntryIDs, "LOCAL_STORAGE_ERROR", now)
		}
	}
	if len(result.missing) != 0 {
		return true, service.requeueContentJob(ctx, job, result.missing, "FOLO_CONTENT_PARTIAL", now)
	}
	if err := jobs.Succeed(ctx, service.store, job.ID, now); err != nil {
		return true, err
	}
	return true, nil
}

func (service *Service) requeueContentJob(ctx context.Context, job jobs.Job, entryIDs []string, code string, now time.Time) error {
	_, err := jobs.Retry(ctx, service.store, job, contentJobPayload{EntryIDs: entryIDs}, code, now, contentJobMaxAttempts)
	return err
}

func decodeContentJobPayload(raw string) (contentJobPayload, error) {
	var payload contentJobPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil || len(payload.EntryIDs) < 1 || len(payload.EntryIDs) > 50 {
		return contentJobPayload{}, errors.New("content job payload is invalid")
	}
	seen := make(map[string]struct{}, len(payload.EntryIDs))
	for _, entryID := range payload.EntryIDs {
		if strings.TrimSpace(entryID) == "" {
			return contentJobPayload{}, errors.New("content job entry ID is invalid")
		}
		if _, duplicate := seen[entryID]; duplicate {
			return contentJobPayload{}, errors.New("content job entry IDs are not unique")
		}
		seen[entryID] = struct{}{}
	}
	return payload, nil
}
