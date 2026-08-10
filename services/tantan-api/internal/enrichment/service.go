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

const maximumActivePoolTranslations = 100

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
			if containsField(fields, "translation") {
				var translations int
				if err := transaction.QueryRowContext(ctx, `
SELECT COUNT(*) FROM entry_enrichments
WHERE entry_id=? AND provider_fp=? AND language=?
  AND translated_title IS NOT NULL AND translated_content IS NOT NULL`, request.EntryID, active.Fingerprint, request.Language).Scan(&translations); err != nil {
					return err
				}
				complete = complete && translations == 1
			}
			if containsField(fields, "summary") || containsField(fields, "keyPoints") {
				var generated int
				if err := transaction.QueryRowContext(ctx, `
SELECT COUNT(*) FROM entry_enrichments
WHERE entry_id=? AND provider_fp=? AND language=?
  AND summary_text IS NOT NULL AND json_array_length(key_points_json)>0`, request.EntryID, active.Fingerprint, request.Language).Scan(&generated); err != nil {
					return err
				}
				complete = complete && generated == 1
			}
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

func (service *Service) EnsureRecentTranslations(ctx context.Context, userID string, limit int) (int, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" || limit < 1 || limit > 100 {
		return 0, errors.New("valid translation warmup request is required")
	}
	now := service.now().UTC()
	rows, err := service.store.DB().QueryContext(ctx, `
SELECT e.entry_id
FROM account_entries ae
JOIN entries e ON e.entry_id=ae.entry_id
WHERE ae.user_id=? AND ae.read_at IS NULL
  AND julianday(e.published_at)>=julianday(?) AND julianday(e.published_at)<=julianday(?)
  AND lower(COALESCE(e.language,'')) NOT LIKE 'zh%'
  AND NOT EXISTS(
    SELECT 1 FROM entry_enrichments en
    WHERE en.entry_id=e.entry_id AND en.state='ready' AND en.content_hash=e.content_hash
      AND en.translated_title IS NOT NULL AND en.translated_content IS NOT NULL
  )
ORDER BY e.published_at DESC,e.entry_id
LIMIT ?`, userID, now.Add(-7*24*time.Hour).Format(time.RFC3339Nano), now.Add(time.Second).Format(time.RFC3339Nano), limit)
	if err != nil {
		return 0, errors.New("load translation warmup entries failed")
	}
	defer rows.Close()
	entryIDs := make([]string, 0, limit)
	for rows.Next() {
		var entryID string
		if err := rows.Scan(&entryID); err != nil {
			return 0, errors.New("scan translation warmup entry failed")
		}
		entryIDs = append(entryIDs, entryID)
	}
	if err := rows.Err(); err != nil {
		return 0, errors.New("iterate translation warmup entries failed")
	}
	return service.ensureTranslationEntries(ctx, userID, entryIDs)
}

// EnsurePoolTranslations incrementally fills the displayable Chinese content pool.
// It considers every synced subscription entry, including old and read entries, but
// only schedules a bounded batch so a large account cannot flood the provider.
func (service *Service) EnsurePoolTranslations(ctx context.Context, userID, sourceID string, limit int) (int, error) {
	userID = strings.TrimSpace(userID)
	sourceID = strings.TrimSpace(sourceID)
	if userID == "" || limit < 1 || limit > 100 {
		return 0, errors.New("valid content pool translation request is required")
	}
	active, _, err := service.settings.Credential(ctx, service.promptVersion)
	if err != nil {
		return 0, err
	}
	var activeCount int
	if err := service.store.DB().QueryRowContext(ctx, `
SELECT COUNT(*)
FROM entry_enrichments en
JOIN account_entries ae ON ae.entry_id=en.entry_id
JOIN entries e ON e.entry_id=en.entry_id
WHERE ae.user_id=? AND en.provider_fp=? AND en.language='zh-CN'
  AND en.content_hash=e.content_hash AND en.prompt_version=?
  AND en.state IN ('queued','processing')`, userID, active.Fingerprint, service.promptVersion).Scan(&activeCount); err != nil {
		return 0, errors.New("count active content pool translations failed")
	}
	available := maximumActivePoolTranslations - activeCount
	if available <= 0 {
		return 0, nil
	}
	if limit > available {
		limit = available
	}
	arguments := []any{userID}
	sourceClause := ""
	if sourceID != "" {
		sourceClause = " AND e.feed_id=?"
		arguments = append(arguments, sourceID)
	}
	retryCutoff := service.now().UTC().Add(-15 * time.Minute).Format(time.RFC3339Nano)
	arguments = append(arguments, active.Fingerprint, service.promptVersion, retryCutoff, limit)
	rows, err := service.store.DB().QueryContext(ctx, `
SELECT e.entry_id
FROM account_entries ae
JOIN entries e ON e.entry_id=ae.entry_id
WHERE ae.user_id=?`+sourceClause+`
  AND lower(COALESCE(e.language,'')) NOT LIKE 'zh%'
  AND NOT EXISTS(
    SELECT 1 FROM entry_enrichments en
    WHERE en.entry_id=e.entry_id AND en.provider_fp=? AND en.language='zh-CN'
      AND en.content_hash=e.content_hash AND en.prompt_version=?
      AND (
        en.state IN ('queued','processing')
        OR (en.state='ready' AND en.translated_title IS NOT NULL AND en.translated_content IS NOT NULL)
        OR (en.state='failed' AND en.updated_at>=?)
      )
  )
ORDER BY e.published_at DESC,e.entry_id
LIMIT ?`, arguments...)
	if err != nil {
		return 0, errors.New("load content pool translation entries failed")
	}
	defer rows.Close()
	entryIDs := make([]string, 0, limit)
	for rows.Next() {
		var entryID string
		if err := rows.Scan(&entryID); err != nil {
			return 0, errors.New("scan content pool translation entry failed")
		}
		entryIDs = append(entryIDs, entryID)
	}
	if err := rows.Err(); err != nil {
		return 0, errors.New("iterate content pool translation entries failed")
	}
	return service.ensureTranslationEntries(ctx, userID, entryIDs)
}

func (service *Service) EnsureQueueTranslations(ctx context.Context, userID, queueID string, limit int) (int, error) {
	userID = strings.TrimSpace(userID)
	queueID = strings.TrimSpace(queueID)
	if userID == "" || queueID == "" || limit < 1 || limit > 100 {
		return 0, errors.New("valid queue translation request is required")
	}
	rows, err := service.store.DB().QueryContext(ctx, `
SELECT e.entry_id
FROM daily_queue_items qi
JOIN daily_queues q ON q.queue_id=qi.queue_id
JOIN entries e ON e.entry_id=qi.entry_id
JOIN account_entries ae ON ae.entry_id=e.entry_id AND ae.user_id=q.user_id
WHERE q.queue_id=? AND q.user_id=? AND qi.state='unread' AND ae.read_at IS NULL
  AND lower(COALESCE(e.language,'')) NOT LIKE 'zh%'
  AND NOT EXISTS(
    SELECT 1 FROM entry_enrichments en
    WHERE en.entry_id=e.entry_id AND en.state='ready' AND en.content_hash=e.content_hash
      AND en.translated_title IS NOT NULL AND en.translated_content IS NOT NULL
  )
ORDER BY qi.rank LIMIT ?`, queueID, userID, limit)
	if err != nil {
		return 0, errors.New("load queue translation entries failed")
	}
	defer rows.Close()
	entryIDs := make([]string, 0, limit)
	for rows.Next() {
		var entryID string
		if err := rows.Scan(&entryID); err != nil {
			return 0, errors.New("scan queue translation entry failed")
		}
		entryIDs = append(entryIDs, entryID)
	}
	if err := rows.Err(); err != nil {
		return 0, errors.New("iterate queue translation entries failed")
	}
	return service.ensureTranslationEntries(ctx, userID, entryIDs)
}

func (service *Service) ensureTranslationEntries(ctx context.Context, userID string, entryIDs []string) (int, error) {
	queued := 0
	for _, entryID := range entryIDs {
		if _, err := service.Ensure(ctx, EnsureRequest{
			UserID:   userID,
			EntryID:  entryID,
			Language: "zh-CN",
			Fields:   []string{"translation", "summary", "keyPoints"},
		}); err != nil {
			return queued, err
		}
		queued++
	}
	return queued, nil
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
