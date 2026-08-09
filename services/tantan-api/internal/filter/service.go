package filter

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
	"sync"
	"time"
	"unicode/utf8"

	"tantan.local/tantan-api/internal/ai"
	"tantan.local/tantan-api/internal/home"
	"tantan.local/tantan-api/internal/recommendation"
	"tantan.local/tantan-api/internal/search"
	"tantan.local/tantan-api/internal/storage"
	"tantan.local/tantan-api/internal/topic"
)

var (
	ErrAIOutputInvalid     = errors.New("AI_OUTPUT_INVALID")
	ErrIdempotencyConflict = errors.New("IDEMPOTENCY_CONFLICT")
	idempotencyKeyPattern  = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
)

type Config struct {
	Store     *storage.Store
	Settings  *ai.SettingsService
	Generator ai.Generator
	Home      *home.Service
	Topics    *topic.Service
	Now       func() time.Time
}

type Service struct {
	store         *storage.Store
	settings      *ai.SettingsService
	generator     ai.Generator
	home          *home.Service
	topics        *topic.Service
	indexer       *search.Indexer
	now           func() time.Time
	mutationMutex sync.Mutex
}

type Request struct {
	UserID         string
	Prompt         string
	Timezone       string
	IdempotencyKey string
}

type State struct {
	ID        string `json:"id"`
	Prompt    string `json:"prompt"`
	CreatedAt string `json:"createdAt"`
}

type Mutation struct {
	Filter  *State       `json:"filter"`
	Topics  []topic.Item `json:"topics"`
	QueueID string       `json:"queueId"`
}

func NewService(config Config) (*Service, error) {
	if config.Store == nil || config.Settings == nil || config.Home == nil || config.Topics == nil {
		return nil, errors.New("filter storage, settings, home, and topics are required")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Service{store: config.Store, settings: config.Settings, generator: config.Generator, home: config.Home, topics: config.Topics, indexer: search.NewIndexer(config.Store), now: now}, nil
}

func (service *Service) Put(ctx context.Context, request Request) (Mutation, error) {
	service.mutationMutex.Lock()
	defer service.mutationMutex.Unlock()

	request.UserID = strings.TrimSpace(request.UserID)
	request.Prompt = strings.TrimSpace(request.Prompt)
	request.Timezone = strings.TrimSpace(request.Timezone)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.Timezone == "" {
		request.Timezone = "Asia/Shanghai"
	}
	if request.UserID == "" || !utf8.ValidString(request.Prompt) {
		return Mutation{}, errors.New("filter user and prompt are required")
	}
	if count := utf8.RuneCountInString(request.Prompt); count < 1 || count > 300 {
		return Mutation{}, errors.New("filter prompt must contain 1 to 300 characters")
	}
	if len(request.IdempotencyKey) < 16 || len(request.IdempotencyKey) > 128 || !idempotencyKeyPattern.MatchString(request.IdempotencyKey) {
		return Mutation{}, errors.New("valid filter idempotency key is required")
	}
	filterID := filterIDFor(request.UserID, request.IdempotencyKey)
	if existing, found, err := service.replay(ctx, request, filterID); err != nil {
		return Mutation{}, err
	} else if found {
		return existing, nil
	}
	active, apiKey, err := service.settings.Credential(ctx, ai.DefaultPromptVersion)
	if err != nil {
		return Mutation{}, err
	}
	generator := service.generator
	if generator == nil {
		generator, err = ai.NewProviderClient(ai.ProviderClientConfig{ProviderID: active.ProviderID, Model: active.Model})
		if err != nil {
			return Mutation{}, err
		}
	}
	generation, err := service.filterPrompt(ctx, request)
	if err != nil {
		return Mutation{}, err
	}
	contents, err := generator.Generate(ctx, apiKey, generation)
	if err != nil {
		return Mutation{}, err
	}
	spec, canonical, validationErr := recommendation.ValidateFilterSpec(contents)
	if validationErr != nil {
		repair := repairFilterPrompt(contents)
		contents, err = generator.Generate(ctx, apiKey, repair)
		if err != nil {
			return Mutation{}, err
		}
		spec, canonical, validationErr = recommendation.ValidateFilterSpec(contents)
	}
	if validationErr != nil || service.validateReferences(ctx, request.UserID, spec) != nil {
		return Mutation{}, ErrAIOutputInvalid
	}
	plan, err := service.home.Plan(ctx, home.PlanRequest{UserID: request.UserID, Timezone: request.Timezone, FilterKey: filterID, Spec: &spec})
	if err != nil {
		return Mutation{}, err
	}
	now := service.now().UTC()
	state := State{ID: filterID, Prompt: request.Prompt, CreatedAt: now.Format(time.RFC3339Nano)}
	var queue home.QueueState
	err = service.store.Write(ctx, func(transaction *sql.Tx) error {
		oldEntries, err := filterTopicEntryIDs(ctx, transaction, request.UserID)
		if err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE home_filters SET status='inactive',updated_at=? WHERE user_id=? AND status='active'", now.Format(time.RFC3339Nano), request.UserID); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, "DELETE FROM topics WHERE user_id=? AND kind='filter'", request.UserID); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, `
INSERT INTO home_filters(filter_id,user_id,prompt,normalized_json,status,created_at,updated_at)
VALUES(?,?,?,?,'active',?,?)`, filterID, request.UserID, request.Prompt, string(canonical), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		queue, err = service.home.ReplaceTx(ctx, transaction, plan)
		if err != nil {
			return err
		}
		generated, err := service.topics.EnsureGeneratedTx(ctx, transaction, request.UserID, filterTopicName(request.Prompt, spec), "filter")
		if err != nil {
			return err
		}
		newEntries := make([]string, 0, len(plan.Items))
		if generated.Kind == "filter" {
			for _, item := range plan.Items {
				result, err := transaction.ExecContext(ctx, `
INSERT OR IGNORE INTO entry_topics(user_id,entry_id,topic_id,confidence,is_primary,content_hash,created_at)
SELECT ?,e.entry_id,?,1,0,e.content_hash,? FROM entries e WHERE e.entry_id=?`, request.UserID, generated.ID, now.Format(time.RFC3339Nano), item.EntryID)
				if err != nil {
					return err
				}
				if affected, _ := result.RowsAffected(); affected > 0 {
					newEntries = append(newEntries, item.EntryID)
				}
			}
		}
		return service.indexer.RefreshTx(ctx, transaction, request.UserID, uniqueSorted(append(oldEntries, newEntries...)))
	})
	if err != nil {
		return Mutation{}, err
	}
	listed, err := service.topics.List(ctx, request.UserID)
	if err != nil {
		return Mutation{}, err
	}
	return Mutation{Filter: &state, Topics: listed.Topics, QueueID: queue.ID}, nil
}

func (service *Service) replay(ctx context.Context, request Request, filterID string) (Mutation, bool, error) {
	var state State
	var status string
	err := service.store.DB().QueryRowContext(ctx, `
SELECT filter_id,prompt,status,created_at FROM home_filters
WHERE filter_id=? AND user_id=?`, filterID, request.UserID).Scan(&state.ID, &state.Prompt, &status, &state.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Mutation{}, false, nil
	}
	if err != nil {
		return Mutation{}, false, err
	}
	if state.Prompt != request.Prompt || status != "active" {
		return Mutation{}, false, ErrIdempotencyConflict
	}
	var queueID string
	var timezone string
	err = service.store.DB().QueryRowContext(ctx, `
SELECT queue_id,timezone FROM daily_queues
WHERE user_id=? AND filter_key=? AND status='ready'
ORDER BY local_date DESC,version DESC LIMIT 1`, request.UserID, filterID).Scan(&queueID, &timezone)
	if errors.Is(err, sql.ErrNoRows) {
		return Mutation{}, false, ErrIdempotencyConflict
	}
	if err != nil {
		return Mutation{}, false, err
	}
	if timezone != request.Timezone {
		return Mutation{}, false, ErrIdempotencyConflict
	}
	listed, err := service.topics.List(ctx, request.UserID)
	if err != nil {
		return Mutation{}, false, err
	}
	return Mutation{Filter: &state, Topics: listed.Topics, QueueID: queueID}, true, nil
}

func (service *Service) Delete(ctx context.Context, userID, timezone string) (Mutation, error) {
	service.mutationMutex.Lock()
	defer service.mutationMutex.Unlock()

	userID = strings.TrimSpace(userID)
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	if userID == "" {
		return Mutation{}, errors.New("filter user is required")
	}
	plan, err := service.home.Plan(ctx, home.PlanRequest{UserID: userID, Timezone: timezone, FilterKey: "default"})
	if err != nil {
		return Mutation{}, err
	}
	now := service.now().UTC()
	var queue home.QueueState
	err = service.store.Write(ctx, func(transaction *sql.Tx) error {
		oldEntries, err := filterTopicEntryIDs(ctx, transaction, userID)
		if err != nil {
			return err
		}
		queue, err = service.home.EnsurePlanTx(ctx, transaction, plan)
		if err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE home_filters SET status='inactive',updated_at=? WHERE user_id=? AND status='active'", now.Format(time.RFC3339Nano), userID); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, "DELETE FROM topics WHERE user_id=? AND kind='filter'", userID); err != nil {
			return err
		}
		return service.indexer.RefreshTx(ctx, transaction, userID, oldEntries)
	})
	if err != nil {
		return Mutation{}, err
	}
	listed, err := service.topics.List(ctx, userID)
	if err != nil {
		return Mutation{}, err
	}
	return Mutation{Filter: nil, Topics: listed.Topics, QueueID: queue.ID}, nil
}

func (service *Service) filterPrompt(ctx context.Context, request Request) (ai.GenerationRequest, error) {
	listed, err := service.topics.List(ctx, request.UserID)
	if err != nil {
		return ai.GenerationRequest{}, err
	}
	availableTopics := make([]map[string]string, 0, len(listed.Topics))
	for _, item := range listed.Topics {
		if item.Kind != "virtual" && item.Kind != "filter" && !item.Hidden {
			availableTopics = append(availableTopics, map[string]string{"id": item.ID, "name": item.Name})
		}
	}
	rows, err := service.store.DB().QueryContext(ctx, `
SELECT DISTINCT f.feed_id,f.title FROM feeds f
JOIN entries e ON e.feed_id=f.feed_id JOIN account_entries ae ON ae.entry_id=e.entry_id
WHERE ae.user_id=? ORDER BY f.feed_id LIMIT 200`, request.UserID)
	if err != nil {
		return ai.GenerationRequest{}, err
	}
	defer rows.Close()
	availableSources := make([]map[string]string, 0, 200)
	for rows.Next() {
		var id string
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return ai.GenerationRequest{}, err
		}
		availableSources = append(availableSources, map[string]string{"id": id, "name": name})
	}
	payload, _ := json.Marshal(map[string]any{"prompt": request.Prompt, "availableTopics": availableTopics, "availableSources": availableSources})
	return ai.GenerationRequest{
		SchemaName:   "filter-spec-v1",
		SystemPrompt: "Tantan prompt-v1. Convert the preference to exactly one FilterSpecV1 JSON object. Use only provided topic/source IDs. No extra keys, Markdown, HTML, URLs, tools, or explanation.",
		UserPrompt:   string(payload),
	}, rows.Err()
}

func repairFilterPrompt(invalid []byte) ai.GenerationRequest {
	if len(invalid) > 200_000 {
		invalid = invalid[:200_000]
	}
	payload, _ := json.Marshal(map[string]string{"schema": "filter-spec-v1", "invalidOutput": string(invalid)})
	return ai.GenerationRequest{SchemaName: "filter-spec-v1", SystemPrompt: "Repair the supplied output once. Return only one valid FilterSpecV1 JSON object, with no Markdown or explanation.", UserPrompt: string(payload), Repair: true}
}

func (service *Service) validateReferences(ctx context.Context, userID string, spec recommendation.FilterSpecV1) error {
	for _, topicID := range append(append([]string{}, spec.IncludeTopics...), spec.ExcludeTopics...) {
		var count int
		if err := service.store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM topics WHERE user_id=? AND topic_id=? AND kind<>'filter'", userID, topicID).Scan(&count); err != nil || count != 1 {
			return ErrAIOutputInvalid
		}
	}
	for _, sourceID := range append(append([]string{}, spec.IncludeSources...), spec.ExcludeSources...) {
		var count int
		if err := service.store.DB().QueryRowContext(ctx, `
SELECT COUNT(*) FROM feeds f WHERE f.feed_id=? AND EXISTS(
  SELECT 1 FROM entries e JOIN account_entries ae ON ae.entry_id=e.entry_id
  WHERE e.feed_id=f.feed_id AND ae.user_id=?
)`, sourceID, userID).Scan(&count); err != nil || count != 1 {
			return ErrAIOutputInvalid
		}
	}
	return nil
}

func filterTopicEntryIDs(ctx context.Context, transaction *sql.Tx, userID string) ([]string, error) {
	rows, err := transaction.QueryContext(ctx, `
SELECT DISTINCT et.entry_id FROM entry_topics et
JOIN topics t ON t.topic_id=et.topic_id AND t.user_id=et.user_id
WHERE t.user_id=? AND t.kind='filter'`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var entryID string
		if err := rows.Scan(&entryID); err != nil {
			return nil, err
		}
		result = append(result, entryID)
	}
	return result, rows.Err()
}

func filterTopicName(prompt string, spec recommendation.FilterSpecV1) string {
	name := prompt
	if len(spec.IncludeTerms) > 0 {
		name = spec.IncludeTerms[0]
	}
	name = strings.TrimSpace(name)
	runes := []rune(name)
	if len(runes) > 20 {
		name = string(runes[:20])
	}
	return name
}

func filterIDFor(userID, idempotencyKey string) string {
	digest := sha256.Sum256([]byte(userID + "\x00" + idempotencyKey))
	return "filter_" + hex.EncodeToString(digest[:16])
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
