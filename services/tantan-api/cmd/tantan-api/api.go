package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime"
	stdhttp "net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"tantan.local/tantan-api/internal/ai"
	"tantan.local/tantan-api/internal/api/gen"
	"tantan.local/tantan-api/internal/contentpool"
	"tantan.local/tantan-api/internal/enrichment"
	"tantan.local/tantan-api/internal/filter"
	"tantan.local/tantan-api/internal/home"
	localhttp "tantan.local/tantan-api/internal/http"
	"tantan.local/tantan-api/internal/jobs"
	"tantan.local/tantan-api/internal/recommendation"
	"tantan.local/tantan-api/internal/search"
	"tantan.local/tantan-api/internal/session"
	"tantan.local/tantan-api/internal/storage"
	"tantan.local/tantan-api/internal/topic"
)

const maximumLocalRequestBytes = 2 * 1024 * 1024

var (
	apiIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)
	idempotencyPattern   = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
)

type providerTester func(context.Context) (ai.ConnectionTestResult, error)

type localAPIConfig struct {
	Store          *storage.Store
	Home           *home.Service
	ContentPool    *contentpool.Service
	Topics         *topic.Service
	Filter         *filter.Service
	Feedback       *recommendation.FeedbackService
	Search         *search.Service
	Enrichment     *enrichment.Service
	AISettings     *ai.SettingsService
	ProviderTester providerTester
	Diagnostics    stdhttp.Handler
	Now            func() time.Time
}

type localAPI struct {
	config       localAPIConfig
	providerLock sync.Mutex
	providerRuns map[string][]time.Time
}

func newLocalAPI(config localAPIConfig) (*localhttp.LocalMux, error) {
	if config.Store == nil || config.Home == nil || config.ContentPool == nil || config.Topics == nil || config.Filter == nil || config.Feedback == nil || config.Search == nil || config.Enrichment == nil || config.AISettings == nil || config.ProviderTester == nil || config.Diagnostics == nil {
		return nil, errors.New("all local API services are required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	api := &localAPI{config: config, providerRuns: make(map[string][]time.Time)}
	mux := localhttp.NewLocalMux()
	mux.HandleFunc(stdhttp.MethodGet, "/tantan/v1/home", api.home)
	mux.HandleFunc(stdhttp.MethodGet, "/tantan/v1/content-pool", api.contentPool)
	mux.HandleFunc(stdhttp.MethodGet, "/tantan/v1/topics", api.getTopics)
	mux.HandleFunc(stdhttp.MethodPatch, "/tantan/v1/topics", api.patchTopics)
	mux.HandleFunc(stdhttp.MethodPut, "/tantan/v1/filter", api.putFilter)
	mux.HandleFunc(stdhttp.MethodDelete, "/tantan/v1/filter", api.deleteFilter)
	mux.HandleFunc(stdhttp.MethodPost, "/tantan/v1/recommendation/feedback", api.feedback)
	mux.HandleFunc(stdhttp.MethodGet, "/tantan/v1/recommendation/blocks/sources", api.listSourceBlocks)
	mux.HandleFunc(stdhttp.MethodDelete, "/tantan/v1/recommendation/blocks/sources/{sourceId}", api.restoreSourceBlock)
	mux.HandleFunc(stdhttp.MethodGet, "/tantan/v1/search", api.search)
	mux.HandleFunc(stdhttp.MethodGet, "/tantan/v1/entries/{entryId}/enrichment", api.getEnrichment)
	mux.HandleFunc(stdhttp.MethodPost, "/tantan/v1/entries/{entryId}/enrichment", api.ensureEnrichment)
	mux.HandleFunc(stdhttp.MethodGet, "/tantan/v1/settings/ai-provider", api.getAISettings)
	mux.HandleFunc(stdhttp.MethodPost, "/tantan/v1/settings/ai-provider/test", api.testAISettings)
	mux.HandleFunc(stdhttp.MethodGet, "/tantan/v1/sync/status", api.syncStatus)
	mux.HandleFunc(stdhttp.MethodPost, "/tantan/v1/sync", api.triggerSync)
	mux.Handle(stdhttp.MethodGet, "/tantan/v1/diagnostics", config.Diagnostics)
	return mux, nil
}

func (api *localAPI) contentPool(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	record, ok := api.record(writer, request)
	if !ok {
		return
	}
	if !onlyQueryKeys(request.URL.Query(), "sourceId", "cursor", "limit") {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "内容池查询参数无效", nil)
		return
	}
	sourceID := request.URL.Query().Get("sourceId")
	if sourceID != "" && !validIdentifier(sourceID) {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "Source ID 无效", nil)
		return
	}
	limit, err := queryLimit(request.URL.Query().Get("limit"))
	if err != nil {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "limit 必须为 1 到 20", nil)
		return
	}
	if _, err := api.config.Enrichment.EnsurePoolTranslations(request.Context(), record.User.ID, sourceID, 100); err != nil && !errors.Is(err, ai.ErrNotConfigured) {
		api.writeDomainError(writer, request, err)
		return
	}
	page, err := api.config.ContentPool.List(request.Context(), contentpool.Query{
		UserID: record.User.ID, SourceID: sourceID, Limit: limit, Cursor: request.URL.Query().Get("cursor"),
	})
	if err != nil {
		api.writeDomainError(writer, request, err)
		return
	}
	writeLocalJSON(writer, stdhttp.StatusOK, contentPoolResponse(page))
}

func (api *localAPI) home(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	record, ok := api.record(writer, request)
	if !ok {
		return
	}
	if !onlyQueryKeys(request.URL.Query(), "topicId", "filterId", "cursor", "limit") {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "首页查询参数无效", nil)
		return
	}
	limit, err := queryLimit(request.URL.Query().Get("limit"))
	if err != nil {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "limit 必须为 1 到 20", nil)
		return
	}
	page, err := api.config.Home.Get(request.Context(), home.Query{UserID: record.User.ID, TopicID: request.URL.Query().Get("topicId"), FilterID: request.URL.Query().Get("filterId"), Timezone: record.Timezone, Limit: limit, Cursor: request.URL.Query().Get("cursor")})
	if err != nil {
		api.writeDomainError(writer, request, err)
		return
	}
	if _, err := api.config.Enrichment.EnsureQueueTranslations(request.Context(), record.User.ID, page.Queue.ID, 60); err != nil {
		api.writeDomainError(writer, request, err)
		return
	}
	writeLocalJSON(writer, stdhttp.StatusOK, homeResponse(page))
}

func (api *localAPI) getTopics(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	record, ok := api.record(writer, request)
	if !ok {
		return
	}
	result, err := api.config.Topics.List(request.Context(), record.User.ID)
	if err != nil {
		api.writeDomainError(writer, request, err)
		return
	}
	if result.ActiveFilterID == nil {
		if err := api.config.Topics.RefreshGenerated(request.Context(), record.User.ID); err != nil {
			api.writeDomainError(writer, request, err)
			return
		}
		result, err = api.config.Topics.List(request.Context(), record.User.ID)
		if err != nil {
			api.writeDomainError(writer, request, err)
			return
		}
	}
	writeLocalJSON(writer, stdhttp.StatusOK, topicsResponse(result))
}

func (api *localAPI) patchTopics(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	record, ok := api.record(writer, request)
	if !ok {
		return
	}
	var body gen.TopicPatchRequest
	if err := decodeLocalJSON(writer, request, &body); err != nil {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "Topic 修改请求无效", nil)
		return
	}
	operations := make([]topic.Operation, 0, len(body.Operations))
	for _, operation := range body.Operations {
		item := topic.Operation{Op: operation.Operation, TopicID: string(operation.TopicID)}
		if operation.AfterTopicID != nil {
			value := string(*operation.AfterTopicID)
			item.AfterTopicID = &value
		}
		operations = append(operations, item)
	}
	result, err := api.config.Topics.Patch(request.Context(), record.User.ID, int64(body.Version), operations)
	if err != nil {
		if errors.Is(err, topic.ErrVersionConflict) {
			current, _ := api.config.Topics.List(request.Context(), record.User.ID)
			version := int(current.Version)
			writeLocalError(writer, request, stdhttp.StatusConflict, gen.ErrorCodeVersionConflict, "Topic 已更新，请刷新后重试", &version)
			return
		}
		api.writeDomainError(writer, request, err)
		return
	}
	writeLocalJSON(writer, stdhttp.StatusOK, topicsResponse(result))
}

func (api *localAPI) putFilter(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	record, ok := api.record(writer, request)
	if !ok {
		return
	}
	idempotencyKey := request.Header.Get("Idempotency-Key")
	if !validIdempotencyKey(idempotencyKey) {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "Idempotency-Key 无效", nil)
		return
	}
	var body gen.FilterPutRequest
	if err := decodeLocalJSON(writer, request, &body); err != nil {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "AI 筛选请求无效", nil)
		return
	}
	result, err := api.config.Filter.Put(request.Context(), filter.Request{UserID: record.User.ID, Prompt: body.Prompt, Timezone: record.Timezone, IdempotencyKey: idempotencyKey})
	if err != nil {
		api.writeDomainError(writer, request, err)
		return
	}
	writeLocalJSON(writer, stdhttp.StatusOK, filterResponse(result))
}

func (api *localAPI) deleteFilter(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	record, ok := api.record(writer, request)
	if !ok {
		return
	}
	result, err := api.config.Filter.Delete(request.Context(), record.User.ID, record.Timezone)
	if err != nil {
		api.writeDomainError(writer, request, err)
		return
	}
	writeLocalJSON(writer, stdhttp.StatusOK, filterResponse(result))
}

func (api *localAPI) feedback(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	record, ok := api.record(writer, request)
	if !ok {
		return
	}
	idempotencyKey := request.Header.Get("Idempotency-Key")
	if !validIdempotencyKey(idempotencyKey) {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "Idempotency-Key 无效", nil)
		return
	}
	var body gen.FeedbackRequest
	if err := decodeLocalJSON(writer, request, &body); err != nil {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "反馈请求无效", nil)
		return
	}
	topicID := ""
	if body.TopicID != nil {
		topicID = string(*body.TopicID)
	}
	result, err := api.config.Feedback.Apply(request.Context(), recommendation.FeedbackRequest{UserID: record.User.ID, EntryID: string(body.EntryID), Action: string(body.Action), TopicID: topicID, IdempotencyKey: idempotencyKey})
	if err != nil {
		api.writeDomainError(writer, request, err)
		return
	}
	writeLocalJSON(writer, stdhttp.StatusOK, result)
}

func (api *localAPI) listSourceBlocks(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	record, ok := api.record(writer, request)
	if !ok {
		return
	}
	if !onlyQueryKeys(request.URL.Query()) {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "屏蔽列表参数无效", nil)
		return
	}
	blocks, err := api.config.Feedback.ListSourceBlocks(request.Context(), record.User.ID)
	if err != nil {
		api.writeDomainError(writer, request, err)
		return
	}
	items := make([]gen.SourceBlock, 0, len(blocks))
	for _, block := range blocks {
		items = append(items, gen.SourceBlock{
			SourceID:  gen.Identifier(block.SourceID),
			Name:      block.Name,
			CreatedAt: block.CreatedAt,
		})
	}
	writeLocalJSON(writer, stdhttp.StatusOK, gen.SourceBlocksResponse{Items: items})
}

func (api *localAPI) restoreSourceBlock(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	record, ok := api.record(writer, request)
	if !ok {
		return
	}
	sourceID := request.PathValue("sourceId")
	idempotencyKey := request.Header.Get("Idempotency-Key")
	if !validIdentifier(sourceID) || !validIdempotencyKey(idempotencyKey) {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "Source 恢复请求无效", nil)
		return
	}
	if err := api.config.Feedback.RestoreSourceBlock(request.Context(), recommendation.RestoreSourceBlockRequest{
		UserID:         record.User.ID,
		SourceID:       sourceID,
		IdempotencyKey: idempotencyKey,
	}); err != nil {
		api.writeDomainError(writer, request, err)
		return
	}
	writeLocalJSON(writer, stdhttp.StatusOK, recommendation.FeedbackResult{Applied: true})
}

func (api *localAPI) search(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	record, ok := api.record(writer, request)
	if !ok {
		return
	}
	if !onlyQueryKeys(request.URL.Query(), "q", "cursor", "limit") {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "搜索参数无效", nil)
		return
	}
	limit, err := queryLimit(request.URL.Query().Get("limit"))
	if err != nil {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "limit 必须为 1 到 50", nil)
		return
	}
	page, err := api.config.Search.Search(request.Context(), search.Query{UserID: record.User.ID, Text: request.URL.Query().Get("q"), Limit: limit, Cursor: request.URL.Query().Get("cursor")})
	if err != nil {
		api.writeDomainError(writer, request, err)
		return
	}
	writeLocalJSON(writer, stdhttp.StatusOK, searchResponse(page))
}

func (api *localAPI) getEnrichment(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	record, ok := api.record(writer, request)
	if !ok {
		return
	}
	language := request.URL.Query().Get("language")
	if language == "" {
		language = "zh-CN"
	}
	entryID := request.PathValue("entryId")
	if !validIdentifier(entryID) {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "Entry ID 无效", nil)
		return
	}
	result, err := api.config.Enrichment.Get(request.Context(), record.User.ID, entryID, language)
	if err != nil {
		api.writeDomainError(writer, request, err)
		return
	}
	data := json.RawMessage("null")
	if result.Data != nil {
		data, err = json.Marshal(result.Data)
		if err != nil {
			api.writeDomainError(writer, request, err)
			return
		}
	}
	var responseError *gen.ErrorObject
	if result.ErrorCode != "" {
		responseError = &gen.ErrorObject{Code: gen.ErrorCode(publicLocalErrorCode(result.ErrorCode)), Message: "AI 处理未完成，请重试"}
	}
	writeLocalJSON(writer, stdhttp.StatusOK, gen.EnrichmentResponse{State: result.State, Data: data, Error: responseError})
}

func (api *localAPI) ensureEnrichment(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	record, ok := api.record(writer, request)
	if !ok {
		return
	}
	if !validIdempotencyKey(request.Header.Get("Idempotency-Key")) {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "Idempotency-Key 无效", nil)
		return
	}
	entryID := request.PathValue("entryId")
	if !validIdentifier(entryID) {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "Entry ID 无效", nil)
		return
	}
	var body gen.EnrichmentEnsureRequest
	if err := decodeLocalJSON(writer, request, &body); err != nil {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "AI 处理请求无效", nil)
		return
	}
	accepted, err := api.config.Enrichment.Ensure(request.Context(), enrichment.EnsureRequest{UserID: record.User.ID, EntryID: entryID, Language: body.Language, Fields: body.Fields})
	if err != nil {
		api.writeDomainError(writer, request, err)
		return
	}
	writeLocalJSON(writer, stdhttp.StatusAccepted, gen.JobAcceptedResponse{JobID: gen.Identifier(accepted.JobID), State: "queued"})
}

func (api *localAPI) getAISettings(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	if _, ok := api.record(writer, request); !ok {
		return
	}
	settings, err := api.config.AISettings.Get(request.Context())
	if err != nil {
		api.writeDomainError(writer, request, err)
		return
	}
	writeLocalJSON(writer, stdhttp.StatusOK, aiSettingsResponse(settings))
}

func (api *localAPI) testAISettings(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	record, ok := api.record(writer, request)
	if !ok {
		return
	}
	if err := requireEmptyBody(request); err != nil {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "AI 测试请求不能包含配置", nil)
		return
	}
	if retryAfter, allowed := api.allowProviderTest(record.IDHash); !allowed {
		writeLocalRateLimit(writer, request, retryAfter)
		return
	}
	result, err := api.config.ProviderTester(request.Context())
	if err != nil {
		api.writeDomainError(writer, request, err)
		return
	}
	writeLocalJSON(writer, stdhttp.StatusOK, gen.AIProviderTestResponse{OK: result.OK, LatencyMS: int(result.LatencyMS), Model: result.Model})
}

func requireEmptyBody(request *stdhttp.Request) error {
	contents, err := io.ReadAll(io.LimitReader(request.Body, 1))
	if err != nil || len(contents) != 0 {
		return errors.New("request body must be empty")
	}
	return nil
}

func (api *localAPI) syncStatus(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	record, ok := api.record(writer, request)
	if !ok {
		return
	}
	response, err := api.readSyncStatus(request.Context(), record.User.ID)
	if err != nil {
		api.writeDomainError(writer, request, err)
		return
	}
	writeLocalJSON(writer, stdhttp.StatusOK, response)
}

func (api *localAPI) triggerSync(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	record, ok := api.record(writer, request)
	if !ok {
		return
	}
	var body gen.SyncTriggerRequest
	if err := decodeLocalJSON(writer, request, &body); err != nil || !validSyncScope(body.Scope) {
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "同步范围无效", nil)
		return
	}
	job, state, err := enqueueSync(request.Context(), api.config.Store, record.User.ID, body.Scope, api.config.Now().UTC())
	if err != nil {
		api.writeDomainError(writer, request, err)
		return
	}
	writeLocalJSON(writer, stdhttp.StatusAccepted, gen.JobAcceptedResponse{JobID: gen.Identifier(job.ID), State: state})
}

func enqueueSync(ctx context.Context, store *storage.Store, userID, scope string, now time.Time) (jobs.Job, string, error) {
	var job jobs.Job
	var state string
	err := store.Write(ctx, func(transaction *sql.Tx) error {
		var err error
		job, err = jobs.EnqueueTx(ctx, transaction, jobs.EnqueueRequest{UserID: userID, Kind: "sync", DedupeKey: "sync:" + userID + ":" + scope, Payload: map[string]string{"scope": scope}, Now: now})
		if err != nil {
			return err
		}
		if err := transaction.QueryRowContext(ctx, "SELECT state FROM jobs WHERE job_id=?", job.ID).Scan(&state); err != nil {
			return err
		}
		_, err = transaction.ExecContext(ctx, `
INSERT INTO sync_state(user_id,state,scope,total,processed,failed,updated_at)
VALUES(?, ?, ?,0,0,0,?)
ON CONFLICT(user_id) DO UPDATE SET
  state=excluded.state,
  scope=excluded.scope,
  error_code=NULL,
	updated_at=excluded.updated_at`, userID, state, scope, now.UTC().Format(time.RFC3339Nano))
		return err
	})
	return job, state, err
}

func (api *localAPI) readSyncStatus(ctx context.Context, userID string) (gen.SyncStatusResponse, error) {
	result := gen.SyncStatusResponse{State: "idle", Counts: gen.SyncCounts{}, UpdatedAt: api.config.Now().UTC().Format(time.RFC3339Nano)}
	var scope sql.NullString
	var errorCode sql.NullString
	err := api.config.Store.DB().QueryRowContext(ctx, "SELECT state,scope,processed,total,failed,error_code,updated_at FROM sync_state WHERE user_id=?", userID).Scan(&result.State, &scope, &result.Counts.Processed, &result.Counts.Total, &result.Counts.Failed, &errorCode, &result.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return gen.SyncStatusResponse{}, err
	}
	if scope.Valid {
		result.Scope = &scope.String
	}
	if errorCode.Valid {
		result.Error = &gen.ErrorObject{Code: gen.ErrorCode(publicLocalErrorCode(errorCode.String)), Message: "同步任务未完成，请稍后重试"}
	}
	return result, nil
}

func (api *localAPI) record(writer stdhttp.ResponseWriter, request *stdhttp.Request) (session.Record, bool) {
	record, ok := session.FromContext(request.Context())
	if !ok {
		writeLocalError(writer, request, stdhttp.StatusUnauthorized, gen.ErrorCodeAuthRequired, "请先登录", nil)
	}
	return record, ok
}

func (api *localAPI) allowProviderTest(sessionID string) (time.Duration, bool) {
	api.providerLock.Lock()
	defer api.providerLock.Unlock()
	now := api.config.Now().UTC()
	cutoff := now.Add(-time.Minute)
	previous := api.providerRuns[sessionID]
	retained := previous[:0]
	for _, item := range previous {
		if item.After(cutoff) {
			retained = append(retained, item)
		}
	}
	if len(retained) >= 3 {
		return retained[0].Add(time.Minute).Sub(now), false
	}
	api.providerRuns[sessionID] = append(retained, now)
	return 0, true
}

func (api *localAPI) writeDomainError(writer stdhttp.ResponseWriter, request *stdhttp.Request, err error) {
	switch {
	case errors.Is(err, home.ErrCursorMismatch), errors.Is(err, home.ErrCursorInvalid), errors.Is(err, search.ErrCursorInvalid), errors.Is(err, search.ErrCursorMismatch), errors.Is(err, contentpool.ErrCursorInvalid), errors.Is(err, contentpool.ErrCursorMismatch):
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeCursorMismatch, "分页游标与当前请求不匹配", nil)
	case errors.Is(err, home.ErrQueueVersionChanged):
		writeLocalError(writer, request, stdhttp.StatusConflict, gen.ErrorCodeQueueVersionChanged, "首页队列已更新，请从第一页重新加载", nil)
	case errors.Is(err, ai.ErrNotConfigured):
		writeLocalError(writer, request, stdhttp.StatusConflict, gen.ErrorCodeAiNotConfigured, "请先配置本机 AI Provider", nil)
	case errors.Is(err, ai.ErrProviderUnavailable):
		writeLocalError(writer, request, stdhttp.StatusBadGateway, gen.ErrorCodeAiProviderUnavailable, "AI Provider 暂时不可用", nil)
	case errors.Is(err, filter.ErrAIOutputInvalid):
		writeLocalError(writer, request, stdhttp.StatusUnprocessableEntity, gen.ErrorCodeAiOutputInvalid, "AI 输出未通过安全校验", nil)
	case errors.Is(err, filter.ErrIdempotencyConflict), errors.Is(err, recommendation.ErrIdempotencyConflict):
		writeLocalError(writer, request, stdhttp.StatusConflict, gen.ErrorCodeVersionConflict, "幂等键已用于其他请求", nil)
	case strings.Contains(strings.ToLower(err.Error()), "not found"):
		writeLocalError(writer, request, stdhttp.StatusNotFound, gen.ErrorCodeNotFound, "请求的数据不存在", nil)
	case isValidationDomainError(err):
		writeLocalError(writer, request, stdhttp.StatusBadRequest, gen.ErrorCodeValidationError, "请求参数无效", nil)
	default:
		writeLocalError(writer, request, stdhttp.StatusInternalServerError, gen.ErrorCodeLocalStorageError, "本地数据暂不可用", nil)
	}
}

func decodeLocalJSON(writer stdhttp.ResponseWriter, request *stdhttp.Request, target any) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("Content-Type must be application/json")
	}
	request.Body = stdhttp.MaxBytesReader(writer, request.Body, maximumLocalRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func writeLocalJSON(writer stdhttp.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeLocalError(writer stdhttp.ResponseWriter, request *stdhttp.Request, status int, code gen.ErrorCode, message string, currentVersion *int) {
	writeLocalJSON(writer, status, gen.ErrorResponse{RequestID: request.Header.Get("X-Request-Id"), Error: gen.ErrorObject{Code: code, Message: message, CurrentVersion: currentVersion}})
}

func writeLocalRateLimit(writer stdhttp.ResponseWriter, request *stdhttp.Request, retryAfter time.Duration) {
	milliseconds := int(retryAfter.Milliseconds())
	if milliseconds < 1 {
		milliseconds = 1
	}
	writer.Header().Set("Retry-After", strconv.Itoa((milliseconds+999)/1000))
	writeLocalJSON(writer, stdhttp.StatusTooManyRequests, gen.ErrorResponse{RequestID: request.Header.Get("X-Request-Id"), Error: gen.ErrorObject{Code: gen.ErrorCodeRateLimited, Message: "测试请求过于频繁，请稍后重试", RetryAfterMS: &milliseconds}})
}

func queryLimit(raw string) (int, error) {
	if raw == "" {
		return 20, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 20 {
		return 0, errors.New("invalid limit")
	}
	return value, nil
}

func onlyQueryKeys(values url.Values, allowed ...string) bool {
	set := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		set[key] = true
	}
	for key, list := range values {
		if !set[key] || len(list) != 1 {
			return false
		}
	}
	return true
}

func validIdempotencyKey(value string) bool {
	return len(value) >= 16 && len(value) <= 128 && idempotencyPattern.MatchString(value)
}

func validIdentifier(value string) bool {
	return len(value) >= 1 && len(value) <= 128 && apiIdentifierPattern.MatchString(value)
}

func validSyncScope(scope string) bool {
	return scope == "all" || scope == "subscriptions" || scope == "entries" || scope == "reads" || scope == "collections"
}

func isValidationDomainError(err error) bool {
	value := strings.ToLower(err.Error())
	for _, part := range []string{"invalid", "required", "must", "requires", "unsupported", "immutable", "1 to", "contain"} {
		if strings.Contains(value, part) {
			return true
		}
	}
	return false
}

func homeResponse(page home.Page) gen.HomeResponse {
	items := make([]gen.HomeCard, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, homeCard(item.EntryID, item.Type, item.Title, item.Excerpt, safeAPIURL(item.Cover), item.Source.ID, item.Source.Name, safeAPIURL(item.Source.Avatar), item.PublishedAt, homeTopics(item.Topics), item.Translated))
	}
	return gen.HomeResponse{Items: items, NextCursor: page.NextCursor, Queue: gen.QueueState{ID: gen.Identifier(page.Queue.ID), Version: page.Queue.Version, Generation: gen.Identifier(page.Queue.Generation), Total: page.Queue.Total, Unread: page.Queue.Unread, Finished: page.Queue.Finished, CandidateWindowDays: page.Queue.CandidateWindowDays, GeneratedAt: page.Queue.GeneratedAt}, QueueGeneration: gen.Identifier(page.QueueGeneration)}
}

func contentPoolResponse(page contentpool.Page) gen.ContentPoolResponse {
	items := make([]gen.HomeCard, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, homeCard(item.EntryID, item.Type, item.Title, item.Excerpt, safeAPIURL(item.Cover), item.Source.ID, item.Source.Name, safeAPIURL(item.Source.Avatar), item.PublishedAt, homeTopics(item.Topics), item.Translated))
	}
	return gen.ContentPoolResponse{
		Items:      items,
		NextCursor: page.NextCursor,
		Pool:       gen.ContentPoolState{Total: page.Pool.Total, Ready: page.Pool.Ready, Pending: page.Pool.Pending},
	}
}

func searchResponse(page search.Page) gen.SearchResponse {
	items := make([]gen.HomeCard, 0, len(page.Items))
	for _, item := range page.Items {
		topics := make([]gen.CardTopic, 0, len(item.Topics))
		for _, value := range item.Topics {
			topics = append(topics, gen.CardTopic{ID: gen.Identifier(value.ID), Name: value.Name})
		}
		items = append(items, homeCard(item.EntryID, item.Kind, item.Title, item.Excerpt, safeAPIURL(item.Cover), item.Source.ID, item.Source.Name, safeAPIURL(item.Source.Avatar), item.PublishedAt, topics, item.Translated))
	}
	return gen.SearchResponse{Items: items, NextCursor: page.NextCursor, IndexStatus: page.IndexStatus}
}

func homeCard(entryID, kind, title string, excerpt, cover *string, sourceID, sourceName string, avatar *string, publishedAt string, topics []gen.CardTopic, translated bool) gen.HomeCard {
	return gen.HomeCard{EntryID: gen.Identifier(entryID), Type: gen.EntryType(kind), Title: title, Excerpt: excerpt, Cover: cover, Source: gen.Source{ID: gen.Identifier(sourceID), Name: sourceName, Avatar: avatar}, PublishedAt: publishedAt, Topics: topics, Translated: translated}
}

func homeTopics(items []home.Topic) []gen.CardTopic {
	result := make([]gen.CardTopic, 0, len(items))
	for _, item := range items {
		result = append(result, gen.CardTopic{ID: gen.Identifier(item.ID), Name: item.Name})
	}
	return result
}

func topicsResponse(response topic.ListResponse) gen.TopicsResponse {
	items := topicItems(response.Topics)
	var active *gen.Identifier
	if response.ActiveFilterID != nil {
		value := gen.Identifier(*response.ActiveFilterID)
		active = &value
	}
	return gen.TopicsResponse{Version: int(response.Version), TopicsRevision: int(response.TopicsRevision), ActiveFilterID: active, Topics: items}
}

func topicItems(items []topic.Item) []gen.Topic {
	result := make([]gen.Topic, 0, len(items))
	for _, item := range items {
		result = append(result, gen.Topic{ID: gen.Identifier(item.ID), Name: item.Name, Kind: item.Kind, Fixed: item.Fixed, Pinned: item.Pinned, Hidden: item.Hidden, UnreadCount: item.UnreadCount})
	}
	return result
}

func filterResponse(response filter.Mutation) gen.FilterMutationResponse {
	result := gen.FilterMutationResponse{Topics: topicItems(response.Topics), TopicsRevision: int(response.TopicsRevision), QueueID: gen.Identifier(response.QueueID), QueueGeneration: gen.Identifier(response.QueueGeneration)}
	if response.Filter != nil {
		result.Filter = &gen.ActiveFilter{ID: gen.Identifier(response.Filter.ID), Prompt: response.Filter.Prompt, CreatedAt: response.Filter.CreatedAt}
	}
	return result
}

func aiSettingsResponse(settings ai.ProviderSettings) gen.AIProviderResponse {
	result := gen.AIProviderResponse{Configured: settings.Configured, HasAPIKey: settings.HasAPIKey}
	if settings.ProviderID != "" {
		providerID := gen.AIProviderID(settings.ProviderID)
		result.ProviderID = &providerID
	}
	if settings.Model != "" {
		result.Model = &settings.Model
	}
	if settings.BaseURL != "" {
		result.BaseURL = &settings.BaseURL
	}
	if settings.KeyFingerprint != "" {
		result.KeyFingerprint = &settings.KeyFingerprint
	}
	return result
}

func safeAPIURL(value *string) *string {
	if value == nil {
		return nil
	}
	parsed, err := url.Parse(*value)
	if err != nil || parsed.User != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil
	}
	return value
}

func publicLocalErrorCode(code string) string {
	allowed := map[string]bool{"AI_NOT_CONFIGURED": true, "AI_PROVIDER_UNAVAILABLE": true, "AI_OUTPUT_INVALID": true, "LOCAL_STORAGE_ERROR": true, "FOLO_UNAVAILABLE": true, "FOLO_RATE_LIMITED": true, "VALIDATION_ERROR": true, "SERVICE_NOT_READY": true}
	if allowed[code] {
		return code
	}
	return "LOCAL_STORAGE_ERROR"
}
