package enrichment

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"tantan.local/tantan-api/internal/ai"
	"tantan.local/tantan-api/internal/jobs"
)

const (
	enrichmentJobLease       = 90 * time.Second
	enrichmentJobMaxAttempts = 2
)

var providerSlots = make(chan struct{}, 2)

var (
	errJobExpanded     = errors.New("enrichment job fields expanded")
	errEnrichmentStale = errors.New("enrichment job is stale")
)

func (service *Service) RunOne(ctx context.Context) (bool, error) {
	now := service.now().UTC()
	job, found, err := jobs.ClaimNext(ctx, service.store, "enrich", now, enrichmentJobLease, enrichmentJobMaxAttempts)
	if err != nil || !found {
		return found, err
	}
	payload, err := decodeJobPayload(job.PayloadJSON)
	if err != nil || payload.UserID != job.UserID {
		return true, service.failTerminal(ctx, job.ID, payload, "JOB_PAYLOAD_INVALID", now)
	}
	active, apiKey, err := service.settings.Credential(ctx, payload.PromptVersion)
	if err != nil {
		if errors.Is(err, ai.ErrNotConfigured) {
			return true, service.failTerminal(ctx, job.ID, payload, "AI_NOT_CONFIGURED", now)
		}
		return true, service.retryProvider(ctx, job, payload, now)
	}
	input, currentHash, err := service.loadEntry(ctx, payload.UserID, payload.EntryID)
	if err != nil {
		return true, service.failTerminal(ctx, job.ID, payload, "ENTRY_NOT_FOUND", now)
	}
	if active.Fingerprint != payload.ProviderFP || currentHash != payload.ContentHash || payload.PromptVersion != service.promptVersion {
		return true, service.cancelStale(ctx, job.ID, payload, now)
	}
	if err := service.setProcessing(ctx, payload, now); err != nil {
		if errors.Is(err, errEnrichmentStale) {
			return true, service.cancelStale(ctx, job.ID, payload, now)
		}
		return true, err
	}
	select {
	case providerSlots <- struct{}{}:
		defer func() { <-providerSlots }()
	case <-ctx.Done():
		return true, ctx.Err()
	}
	generator := service.generator
	if generator == nil {
		generator, err = ai.NewProviderClient(ai.ProviderClientConfig{ProviderID: active.ProviderID, Model: active.Model})
		if err != nil {
			return true, service.failTerminal(ctx, job.ID, payload, "AI_NOT_CONFIGURED", now)
		}
	}
	enrichmentRequest := enrichmentPrompt(input, payload.Language, payload.PromptVersion)
	enrichmentOutput, generationErr, valid := generateEnrichment(ctx, generator, apiKey, enrichmentRequest)
	if generationErr != nil {
		if ai.ShouldRetryProvider(generationErr) {
			return true, service.retryProvider(ctx, job, payload, now)
		}
		return true, service.failTerminal(ctx, job.ID, payload, "AI_PROVIDER_UNAVAILABLE", now)
	}
	if !valid {
		return true, service.failTerminal(ctx, job.ID, payload, "AI_OUTPUT_INVALID", now)
	}
	payload, err = service.currentJobPayload(ctx, job.ID, payload)
	if err != nil {
		return true, err
	}
	var classification *ai.TopicClassificationV1
	if containsField(payload.Fields, "topics") {
		list, err := service.topics.List(ctx, payload.UserID)
		if err != nil {
			return true, err
		}
		topicRequest := topicPrompt(input, list.Topics, payload.PromptVersion)
		topicOutput, generationErr, valid := generateTopics(ctx, generator, apiKey, topicRequest)
		if generationErr != nil {
			if ai.ShouldRetryProvider(generationErr) {
				return true, service.retryProvider(ctx, job, payload, now)
			}
			return true, service.failTerminal(ctx, job.ID, payload, "AI_PROVIDER_UNAVAILABLE", now)
		}
		if !valid {
			return true, service.failTerminal(ctx, job.ID, payload, "AI_OUTPUT_INVALID", now)
		}
		classification = &topicOutput
	}
	if err := service.commitReady(ctx, job.ID, payload, enrichmentOutput, classification, now); err != nil {
		if errors.Is(err, errEnrichmentStale) {
			return true, service.cancelStale(ctx, job.ID, payload, now)
		}
		if !errors.Is(err, errJobExpanded) || classification != nil {
			return true, err
		}
		payload, err = service.currentJobPayload(ctx, job.ID, payload)
		if err != nil {
			return true, err
		}
		list, err := service.topics.List(ctx, payload.UserID)
		if err != nil {
			return true, err
		}
		topicRequest := topicPrompt(input, list.Topics, payload.PromptVersion)
		topicOutput, generationErr, valid := generateTopics(ctx, generator, apiKey, topicRequest)
		if generationErr != nil {
			if ai.ShouldRetryProvider(generationErr) {
				return true, service.retryProvider(ctx, job, payload, now)
			}
			return true, service.failTerminal(ctx, job.ID, payload, "AI_PROVIDER_UNAVAILABLE", now)
		}
		if !valid {
			return true, service.failTerminal(ctx, job.ID, payload, "AI_OUTPUT_INVALID", now)
		}
		classification = &topicOutput
		if err := service.commitReady(ctx, job.ID, payload, enrichmentOutput, classification, now); err != nil {
			if errors.Is(err, errEnrichmentStale) {
				return true, service.cancelStale(ctx, job.ID, payload, now)
			}
			return true, err
		}
	}
	return true, nil
}

func generateEnrichment(ctx context.Context, generator ai.Generator, apiKey string, request ai.GenerationRequest) (ai.EnrichmentV1, error, bool) {
	contents, err := generator.Generate(ctx, apiKey, request)
	if err != nil {
		return ai.EnrichmentV1{}, err, false
	}
	output, err := ai.ValidateEnrichmentOutput(contents)
	if err == nil {
		return output, nil, true
	}
	contents, err = generator.Generate(ctx, apiKey, repairPrompt(request, contents))
	if err != nil {
		return ai.EnrichmentV1{}, err, false
	}
	output, err = ai.ValidateEnrichmentOutput(contents)
	return output, nil, err == nil
}

func generateTopics(ctx context.Context, generator ai.Generator, apiKey string, request ai.GenerationRequest) (ai.TopicClassificationV1, error, bool) {
	contents, err := generator.Generate(ctx, apiKey, request)
	if err != nil {
		return ai.TopicClassificationV1{}, err, false
	}
	output, err := ai.ValidateTopicOutput(contents)
	if err == nil {
		return output, nil, true
	}
	contents, err = generator.Generate(ctx, apiKey, repairPrompt(request, contents))
	if err != nil {
		return ai.TopicClassificationV1{}, err, false
	}
	output, err = ai.ValidateTopicOutput(contents)
	return output, nil, err == nil
}

func (service *Service) loadEntry(ctx context.Context, userID, entryID string) (entryInput, string, error) {
	var input entryInput
	var description sql.NullString
	var content sql.NullString
	var language sql.NullString
	var contentHash string
	err := service.store.DB().QueryRowContext(ctx, `
SELECT e.title,e.description,e.content,e.language,e.content_hash
FROM entries e JOIN account_entries ae ON ae.entry_id=e.entry_id
WHERE ae.user_id=? AND e.entry_id=?`, userID, entryID).Scan(&input.Title, &description, &content, &language, &contentHash)
	if err != nil {
		return entryInput{}, "", err
	}
	input.Description = description.String
	input.Content = content.String
	input.Language = language.String
	return input, contentHash, nil
}

func (service *Service) setProcessing(ctx context.Context, payload jobPayload, now time.Time) error {
	return service.store.Write(ctx, func(transaction *sql.Tx) error {
		result, err := transaction.ExecContext(ctx, `
UPDATE entry_enrichments SET state='processing',error_code=NULL,updated_at=?
WHERE entry_id=? AND provider_fp=? AND language=? AND content_hash=? AND prompt_version=? AND state='queued'`, now.Format(time.RFC3339Nano), payload.EntryID, payload.ProviderFP, payload.Language, payload.ContentHash, payload.PromptVersion)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil || affected != 1 {
			return errEnrichmentStale
		}
		return nil
	})
}

func (service *Service) commitReady(ctx context.Context, jobID string, payload jobPayload, output ai.EnrichmentV1, classification *ai.TopicClassificationV1, now time.Time) error {
	keyPoints, err := json.Marshal(output.KeyPoints)
	if err != nil {
		return errors.New("encode enrichment key points")
	}
	return service.store.Write(ctx, func(transaction *sql.Tx) error {
		var rawPayload string
		if err := transaction.QueryRowContext(ctx, "SELECT payload_json FROM jobs WHERE job_id=? AND state='running'", jobID).Scan(&rawPayload); err != nil {
			return err
		}
		current, err := decodeJobPayload(rawPayload)
		if err != nil {
			return err
		}
		if containsField(current.Fields, "topics") && classification == nil {
			return errJobExpanded
		}
		if err := service.assertCurrentTx(ctx, transaction, payload); err != nil {
			return err
		}
		result, err := transaction.ExecContext(ctx, `
UPDATE entry_enrichments SET
  state='ready',translated_title=?,translated_content=?,summary_text=?,key_points_json=?,tags_json='[]',quality_score=10,
  error_code=NULL,updated_at=?
WHERE entry_id=? AND provider_fp=? AND language=? AND content_hash=? AND prompt_version=? AND state='processing'`, nullable(output.TitleZh), nullable(output.ContentZh), output.SummaryZh, string(keyPoints), now.Format(time.RFC3339Nano), payload.EntryID, payload.ProviderFP, payload.Language, payload.ContentHash, payload.PromptVersion)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil || affected != 1 {
			return errEnrichmentStale
		}
		if classification != nil {
			if err := service.topics.ApplyClassificationTx(ctx, transaction, payload.UserID, payload.EntryID, payload.ContentHash, *classification); err != nil {
				return err
			}
		}
		if err := service.indexer.RefreshTx(ctx, transaction, payload.UserID, []string{payload.EntryID}); err != nil {
			return err
		}
		return jobs.FinishTx(ctx, transaction, jobID, "succeeded", "", now)
	})
}

func (service *Service) assertCurrentTx(ctx context.Context, transaction *sql.Tx, payload jobPayload) error {
	var currentHash string
	if err := transaction.QueryRowContext(ctx, `
SELECT e.content_hash
FROM entries e JOIN account_entries ae ON ae.entry_id=e.entry_id
WHERE ae.user_id=? AND e.entry_id=?`, payload.UserID, payload.EntryID).Scan(&currentHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errEnrichmentStale
		}
		return err
	}
	if currentHash != payload.ContentHash {
		return errEnrichmentStale
	}
	var providerID string
	var model string
	var baseURL string
	if err := transaction.QueryRowContext(ctx, `
SELECT provider_id,model,base_url FROM ai_provider_configs WHERE enabled=1`).Scan(&providerID, &model, &baseURL); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errEnrichmentStale
		}
		return err
	}
	preset, err := ai.ProviderPreset(providerID)
	if err != nil || preset.BaseURL != baseURL {
		return errEnrichmentStale
	}
	fingerprint, err := ai.ProviderFingerprint(providerID, model, payload.PromptVersion, ai.SchemaVersion)
	if err != nil || fingerprint != payload.ProviderFP {
		return errEnrichmentStale
	}
	return nil
}

func (service *Service) currentJobPayload(ctx context.Context, jobID string, original jobPayload) (jobPayload, error) {
	var raw string
	if err := service.store.DB().QueryRowContext(ctx, "SELECT payload_json FROM jobs WHERE job_id=? AND state='running'", jobID).Scan(&raw); err != nil {
		return jobPayload{}, errors.New("read active enrichment job failed")
	}
	current, err := decodeJobPayload(raw)
	if err != nil {
		return jobPayload{}, err
	}
	if current.UserID != original.UserID || current.EntryID != original.EntryID || current.Language != original.Language || current.ProviderFP != original.ProviderFP || current.ContentHash != original.ContentHash || current.PromptVersion != original.PromptVersion {
		return jobPayload{}, errors.New("active enrichment job identity changed")
	}
	return current, nil
}

func (service *Service) retryProvider(ctx context.Context, job jobs.Job, payload jobPayload, now time.Time) error {
	return service.store.Write(ctx, func(transaction *sql.Tx) error {
		terminal, err := jobs.RetryTx(ctx, transaction, job, payload, "AI_PROVIDER_UNAVAILABLE", now, enrichmentJobMaxAttempts)
		if err != nil {
			return err
		}
		state := "queued"
		if terminal {
			state = "failed"
		}
		_, err = transaction.ExecContext(ctx, `
UPDATE entry_enrichments SET state=?,error_code=?,updated_at=?
WHERE entry_id=? AND provider_fp=? AND language=? AND content_hash=? AND prompt_version=?`, state, "AI_PROVIDER_UNAVAILABLE", now.Format(time.RFC3339Nano), payload.EntryID, payload.ProviderFP, payload.Language, payload.ContentHash, payload.PromptVersion)
		return err
	})
}

func (service *Service) failTerminal(ctx context.Context, jobID string, payload jobPayload, code string, now time.Time) error {
	return service.store.Write(ctx, func(transaction *sql.Tx) error {
		if payload.EntryID != "" && payload.ProviderFP != "" && payload.Language != "" {
			if _, err := transaction.ExecContext(ctx, `
UPDATE entry_enrichments SET state='failed',error_code=?,updated_at=?
WHERE entry_id=? AND provider_fp=? AND language=? AND content_hash=? AND prompt_version=?`, code, now.Format(time.RFC3339Nano), payload.EntryID, payload.ProviderFP, payload.Language, payload.ContentHash, payload.PromptVersion); err != nil {
				return err
			}
		}
		return jobs.FinishTx(ctx, transaction, jobID, "failed", code, now)
	})
}

func (service *Service) cancelStale(ctx context.Context, jobID string, payload jobPayload, now time.Time) error {
	return service.store.Write(ctx, func(transaction *sql.Tx) error {
		if payload.EntryID != "" && payload.ProviderFP != "" && payload.Language != "" {
			_, _ = transaction.ExecContext(ctx, `
UPDATE entry_enrichments SET state='stale',updated_at=?
WHERE entry_id=? AND provider_fp=? AND language=? AND content_hash=? AND prompt_version=?
  AND state IN ('queued','processing')`, now.Format(time.RFC3339Nano), payload.EntryID, payload.ProviderFP, payload.Language, payload.ContentHash, payload.PromptVersion)
		}
		return jobs.FinishTx(ctx, transaction, jobID, "cancelled", "JOB_STALE", now)
	})
}

func decodeJobPayload(raw string) (jobPayload, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	var payload jobPayload
	if err := decoder.Decode(&payload); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return jobPayload{}, errors.New("enrichment job payload is invalid")
	}
	if _, err := validateEnsureRequest(EnsureRequest{UserID: payload.UserID, EntryID: payload.EntryID, Language: payload.Language, Fields: payload.Fields}); err != nil || len(payload.ProviderFP) != 12 || len(payload.ContentHash) != 64 || strings.TrimSpace(payload.PromptVersion) == "" {
		return jobPayload{}, errors.New("enrichment job payload is invalid")
	}
	return payload, nil
}

func nullable(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
