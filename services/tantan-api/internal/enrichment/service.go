package enrichment

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"tantan.local/tantan-api/internal/ai"
	"tantan.local/tantan-api/internal/jobs"
	"tantan.local/tantan-api/internal/search"
	"tantan.local/tantan-api/internal/storage"
	"tantan.local/tantan-api/internal/topic"
)

var targetLanguagePattern = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z]{2})?$`)

var allowedFields = map[string]struct{}{
	"translation": {},
	"summary":     {},
	"keyPoints":   {},
	"topics":      {},
}

type Service struct {
	store         *storage.Store
	settings      *ai.SettingsService
	generator     ai.Generator
	topics        *topic.Service
	indexer       *search.Indexer
	now           func() time.Time
	promptVersion string
}

func NewService(config Config) (*Service, error) {
	if config.Store == nil || config.Settings == nil || config.Topics == nil {
		return nil, errors.New("enrichment storage, settings, and topics are required")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	promptVersion := strings.TrimSpace(config.PromptVersion)
	if promptVersion == "" {
		promptVersion = ai.DefaultPromptVersion
	}
	return &Service{
		store:         config.Store,
		settings:      config.Settings,
		generator:     config.Generator,
		topics:        config.Topics,
		indexer:       search.NewIndexer(config.Store),
		now:           now,
		promptVersion: promptVersion,
	}, nil
}

func (service *Service) Ensure(ctx context.Context, request EnsureRequest) (Accepted, error) {
	fields, err := validateEnsureRequest(request)
	if err != nil {
		return Accepted{}, err
	}
	active, _, err := service.settings.Credential(ctx, service.promptVersion)
	if err != nil {
		return Accepted{}, err
	}
	var contentHash string
	err = service.store.DB().QueryRowContext(ctx, `
SELECT e.content_hash
FROM entries e JOIN account_entries ae ON ae.entry_id=e.entry_id
WHERE ae.user_id=? AND e.entry_id=?`, request.UserID, request.EntryID).Scan(&contentHash)
	if errors.Is(err, sql.ErrNoRows) {
		return Accepted{}, errors.New("entry was not found")
	}
	if err != nil {
		return Accepted{}, errors.New("read enrichment entry failed")
	}
	payload := jobPayload{
		UserID:        request.UserID,
		EntryID:       request.EntryID,
		Language:      request.Language,
		Fields:        fields,
		ProviderFP:    active.Fingerprint,
		ContentHash:   contentHash,
		PromptVersion: service.promptVersion,
	}
	dedupeKey := enrichmentDedupe(payload)
	var accepted Accepted
	now := service.now().UTC()
	err = service.store.Write(ctx, func(transaction *sql.Tx) error {
		var state string
		var existingHash string
		var existingPrompt string
		err := transaction.QueryRowContext(ctx, `
SELECT state,content_hash,prompt_version
FROM entry_enrichments
WHERE entry_id=? AND provider_fp=? AND language=?`, request.EntryID, active.Fingerprint, request.Language).Scan(&state, &existingHash, &existingPrompt)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && state == "ready" && existingHash == contentHash && existingPrompt == service.promptVersion {
			complete := true
			if containsField(fields, "topics") {
				var assignments int
				if err := transaction.QueryRowContext(ctx, `
SELECT COUNT(*) FROM entry_topics
WHERE user_id=? AND entry_id=? AND content_hash=?`, request.UserID, request.EntryID, contentHash).Scan(&assignments); err != nil {
					return err
				}
				complete = assignments > 0
			}
			if complete {
				accepted.JobID = "job_ready_" + dedupeKey[:24]
				return nil
			}
		}
		if _, err := transaction.ExecContext(ctx, `
UPDATE entry_enrichments SET state='stale',updated_at=?
WHERE entry_id=? AND language=? AND (
  provider_fp<>? OR content_hash<>? OR prompt_version<>?
) AND state<>'stale'`, now.Format(time.RFC3339Nano), request.EntryID, request.Language, active.Fingerprint, contentHash, service.promptVersion); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, `
INSERT INTO entry_enrichments(
  entry_id,provider_fp,language,state,key_points_json,tags_json,content_hash,prompt_version,created_at,updated_at
) VALUES(?,?,?,'queued','[]','[]',?,?,?,?)
ON CONFLICT(entry_id,provider_fp,language) DO UPDATE SET
	state=CASE
	  WHEN entry_enrichments.state='processing'
	    AND entry_enrichments.content_hash=excluded.content_hash
	    AND entry_enrichments.prompt_version=excluded.prompt_version
	  THEN 'processing'
	  ELSE 'queued'
	END,
  content_hash=excluded.content_hash,
  prompt_version=excluded.prompt_version,
  error_code=NULL,
  updated_at=excluded.updated_at`, request.EntryID, active.Fingerprint, request.Language, contentHash, service.promptVersion, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		job, err := jobs.EnqueueTx(ctx, transaction, jobs.EnqueueRequest{
			UserID:    request.UserID,
			Kind:      "enrich",
			DedupeKey: dedupeKey,
			Payload:   payload,
			Now:       now,
		})
		if err != nil {
			return err
		}
		currentPayload, err := decodeJobPayload(job.PayloadJSON)
		if err != nil {
			return err
		}
		payload.Fields = mergeFields(currentPayload.Fields, payload.Fields)
		encodedPayload, err := json.Marshal(payload)
		if err != nil {
			return errors.New("encode merged enrichment job")
		}
		result, err := transaction.ExecContext(ctx, `
UPDATE jobs SET payload_json=?,updated_at=?
WHERE job_id=? AND state IN ('queued','running')`, string(encodedPayload), now.Format(time.RFC3339Nano), job.ID)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil || affected != 1 {
			return errors.New("merge enrichment job lost")
		}
		accepted.JobID = job.ID
		return nil
	})
	if err != nil {
		return Accepted{}, errors.New("queue enrichment failed")
	}
	return accepted, nil
}

func (service *Service) Get(ctx context.Context, userID, entryID, language string) (Result, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(entryID) == "" || !targetLanguagePattern.MatchString(language) {
		return Result{}, errors.New("valid enrichment lookup is required")
	}
	active, _, err := service.settings.Credential(ctx, service.promptVersion)
	if errors.Is(err, ai.ErrNotConfigured) {
		return Result{State: "missing"}, nil
	}
	if err != nil {
		return Result{}, err
	}
	var state string
	var translatedTitle sql.NullString
	var translatedContent sql.NullString
	var summary sql.NullString
	var keyPointsJSON string
	var errorCode sql.NullString
	var detectedLanguage sql.NullString
	err = service.store.DB().QueryRowContext(ctx, `
SELECT en.state,en.translated_title,en.translated_content,en.summary_text,en.key_points_json,en.error_code,e.language
FROM entry_enrichments en
JOIN entries e ON e.entry_id=en.entry_id
JOIN account_entries ae ON ae.entry_id=e.entry_id AND ae.user_id=?
WHERE en.entry_id=? AND en.provider_fp=? AND en.language=?
  AND en.content_hash=e.content_hash AND en.prompt_version=?`, userID, entryID, active.Fingerprint, language, service.promptVersion).Scan(
		&state,
		&translatedTitle,
		&translatedContent,
		&summary,
		&keyPointsJSON,
		&errorCode,
		&detectedLanguage,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Result{State: "missing"}, nil
	}
	if err != nil {
		return Result{}, errors.New("read enrichment failed")
	}
	if state == "stale" {
		return Result{State: "missing"}, nil
	}
	result := Result{State: state}
	if errorCode.Valid {
		result.ErrorCode = errorCode.String
	}
	if state != "ready" {
		return result, nil
	}
	var keyPoints []string
	if err := json.Unmarshal([]byte(keyPointsJSON), &keyPoints); err != nil {
		return Result{}, errors.New("stored enrichment is invalid")
	}
	detected := detectedLanguage.String
	if !targetLanguagePattern.MatchString(detected) {
		detected = "en"
	}
	data := ai.EnrichmentV1{
		Version:          1,
		DetectedLanguage: detected,
		SummaryZh:        summary.String,
		KeyPoints:        keyPoints,
	}
	if translatedTitle.Valid {
		data.TitleZh = &translatedTitle.String
	}
	if translatedContent.Valid {
		data.ContentZh = &translatedContent.String
	}
	result.Data = &data
	return result, nil
}

func validateEnsureRequest(request EnsureRequest) ([]string, error) {
	if strings.TrimSpace(request.UserID) == "" || strings.TrimSpace(request.EntryID) == "" || !targetLanguagePattern.MatchString(request.Language) || len(request.Fields) < 1 || len(request.Fields) > 4 {
		return nil, errors.New("valid enrichment request is required")
	}
	fields := append([]string(nil), request.Fields...)
	sort.Strings(fields)
	for index, field := range fields {
		if _, ok := allowedFields[field]; !ok || (index > 0 && fields[index-1] == field) {
			return nil, errors.New("enrichment fields are invalid")
		}
	}
	return fields, nil
}

func enrichmentDedupe(payload jobPayload) string {
	payload.Fields = nil
	encoded, _ := json.Marshal(payload)
	digest := sha256.Sum256(encoded)
	return "enrich:" + hex.EncodeToString(digest[:])
}

func mergeFields(left, right []string) []string {
	set := make(map[string]struct{}, len(left)+len(right))
	for _, field := range left {
		set[field] = struct{}{}
	}
	for _, field := range right {
		set[field] = struct{}{}
	}
	merged := make([]string, 0, len(set))
	for field := range set {
		merged = append(merged, field)
	}
	sort.Strings(merged)
	return merged
}

func containsField(fields []string, target string) bool {
	for _, field := range fields {
		if field == target {
			return true
		}
	}
	return false
}
