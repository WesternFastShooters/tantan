package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/keyring"
	"tantan.local/tantan-api/internal/session"
)

type applicationSecrets struct {
	mutex  sync.Mutex
	values map[string]string
}

func (secrets *applicationSecrets) Get(_ context.Context, account string) (string, error) {
	secrets.mutex.Lock()
	defer secrets.mutex.Unlock()
	value, ok := secrets.values[account]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (secrets *applicationSecrets) Set(_ context.Context, account, value string) error {
	secrets.mutex.Lock()
	secrets.values[account] = value
	secrets.mutex.Unlock()
	return nil
}

func (secrets *applicationSecrets) Delete(_ context.Context, account string) error {
	secrets.mutex.Lock()
	delete(secrets.values, account)
	secrets.mutex.Unlock()
	return nil
}

func TestApplicationStartsMigratedReadyAndRoutesLocalAPI(t *testing.T) {
	upstream := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":0,"data":[]}`))
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	secrets := &applicationSecrets{values: map[string]string{}}
	application, err := newApplication(context.Background(), applicationConfig{
		DataDir:       t.TempDir(),
		PublicOrigin:  "http://127.0.0.1:3000",
		Upstream:      upstreamURL,
		FoloWebURL:    upstreamURL,
		Client:        upstream.Client(),
		FoloSecrets:   secrets,
		AISecrets:     secrets,
		ProbeKeychain: secrets,
		CursorSecrets: secrets,
		Logger:        slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Now:           func() time.Time { return time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC) },
		Version:       "test-version",
		StartWorkers:  false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	for _, test := range []struct {
		path   string
		status int
	}{
		{path: "/api/healthz", status: stdhttp.StatusOK},
		{path: "/api/readyz", status: stdhttp.StatusOK},
		{path: "/api/tantan/v1/home?topicId=recommend", status: stdhttp.StatusUnauthorized},
	} {
		request := httptest.NewRequest(stdhttp.MethodGet, "http://127.0.0.1:3000"+test.path, nil)
		request.Host = "127.0.0.1:3000"
		response := httptest.NewRecorder()
		application.Handler.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("%s status=%d body=%s", test.path, response.Code, response.Body.String())
		}
	}
	var migrations int
	if err := application.Store.DB().QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&migrations); err != nil || migrations != 4 {
		t.Fatalf("migrations=%d err=%v", migrations, err)
	}
	if err := application.Close(); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestApplicationAuthenticatedHomeTopicsSearchAndStrictAISettings(t *testing.T) {
	ctx := context.Background()
	upstream := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":0,"data":[]}`))
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	secrets := &applicationSecrets{values: map[string]string{}}
	now := func() time.Time { return time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC) }
	application, err := newApplication(ctx, applicationConfig{
		DataDir:       t.TempDir(),
		PublicOrigin:  "http://127.0.0.1:3000",
		Upstream:      upstreamURL,
		FoloWebURL:    upstreamURL,
		Client:        upstream.Client(),
		FoloSecrets:   secrets,
		AISecrets:     secrets,
		ProbeKeychain: secrets,
		CursorSecrets: secrets,
		Logger:        slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Now:           now,
		Version:       "test-version",
		StartWorkers:  false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()

	backend, err := newSQLiteSessionBackend(application.Store)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := session.NewStoreWithBackend(now, backend)
	if err != nil {
		t.Fatal(err)
	}
	rawSession := strings.Repeat("s", 48)
	csrfToken := strings.Repeat("c", 48)
	if _, err := sessions.CreateWithCSRF(ctx, rawSession, csrfToken, session.User{ID: "user_api", Name: "API User"}, now().Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	timestamp := now().Format(time.RFC3339Nano)
	hash := strings.Repeat("a", 64)
	if err := application.Store.Write(ctx, func(transaction *sql.Tx) error {
		statements := []struct {
			query string
			args  []any
		}{
			{query: "INSERT INTO feeds(feed_id,title,url,image,view,updated_at) VALUES(?,?,?,?,?,?)", args: []any{"feed_api", "API Source", "https://source.invalid/feed", nil, 0, timestamp}},
			{query: "INSERT INTO entries(entry_id,feed_id,kind,title,description,content,url,language,media_json,published_at,content_hash,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)", args: []any{"entry_api", "feed_api", "article", "Contract Needle", "description", "searchable body", "https://content.invalid/item", "en", "[]", timestamp, hash, timestamp, timestamp}},
			{query: "INSERT INTO account_entries(user_id,entry_id,last_seen_at) VALUES(?,?,?)", args: []any{"user_api", "entry_api", timestamp}},
			{query: "INSERT INTO entry_search(entry_id,user_id,title,translation,content,source,topics,tags) VALUES(?,?,?,?,?,?,?,?)", args: []any{"entry_api", "user_api", "Contract Needle", "", "searchable body", "API Source", "", ""}},
		}
		for _, statement := range statements {
			if _, err := transaction.ExecContext(ctx, statement.query, statement.args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	do := func(method, path string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, "http://127.0.0.1:3000"+path, bytes.NewReader(body))
		request.Host = "127.0.0.1:3000"
		request.AddCookie(&stdhttp.Cookie{Name: session.LocalCookieName, Value: rawSession})
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		if method != stdhttp.MethodGet {
			request.Header.Set("Origin", "http://127.0.0.1:3000")
			request.Header.Set("X-CSRF-Token", csrfToken)
		}
		response := httptest.NewRecorder()
		application.Handler.ServeHTTP(response, request)
		return response
	}

	homeResponse := do(stdhttp.MethodGet, "/api/tantan/v1/home?topicId=recommend&limit=20", nil)
	if homeResponse.Code != stdhttp.StatusOK {
		t.Fatalf("home status=%d body=%s", homeResponse.Code, homeResponse.Body.String())
	}
	var homeBody struct {
		Items []struct {
			EntryID string `json:"entryId"`
		} `json:"items"`
		Queue struct {
			ID         string `json:"id"`
			Generation string `json:"generation"`
		} `json:"queue"`
		QueueGeneration string `json:"queueGeneration"`
	}
	if err := json.Unmarshal(homeResponse.Body.Bytes(), &homeBody); err != nil || len(homeBody.Items) != 1 || homeBody.Items[0].EntryID != "entry_api" || homeBody.Queue.ID == "" || homeBody.Queue.Generation == "" || homeBody.QueueGeneration != homeBody.Queue.Generation {
		t.Fatalf("home response=%s err=%v", homeResponse.Body.String(), err)
	}
	queueID := homeBody.Queue.ID
	if response := do(stdhttp.MethodGet, "/api/tantan/v1/home?topicId=recommend&limit=21", nil); response.Code != stdhttp.StatusBadRequest {
		t.Fatalf("home accepted limit=21: status=%d body=%s", response.Code, response.Body.String())
	}

	for _, path := range []string{"/api/tantan/v1/topics", "/api/tantan/v1/search?q=Contract%20Needle&limit=20"} {
		response := do(stdhttp.MethodGet, path, nil)
		if response.Code != stdhttp.StatusOK || !strings.Contains(response.Body.String(), "entry_api") && strings.Contains(path, "/search") {
			t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	afterSearch := do(stdhttp.MethodGet, "/api/tantan/v1/home?topicId=recommend&limit=20", nil)
	if afterSearch.Code != stdhttp.StatusOK || !strings.Contains(afterSearch.Body.String(), `"id":"`+queueID+`"`) {
		t.Fatalf("search mutated home queue: %s", afterSearch.Body.String())
	}
	doWithKey := func(method, path string, body []byte, idempotencyKey string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, "http://127.0.0.1:3000"+path, bytes.NewReader(body))
		request.Host = "127.0.0.1:3000"
		request.AddCookie(&stdhttp.Cookie{Name: session.LocalCookieName, Value: rawSession})
		request.Header.Set("Origin", "http://127.0.0.1:3000")
		request.Header.Set("X-CSRF-Token", csrfToken)
		request.Header.Set("Idempotency-Key", idempotencyKey)
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		application.Handler.ServeHTTP(response, request)
		return response
	}
	blocked := doWithKey(stdhttp.MethodPost, "/api/tantan/v1/recommendation/feedback", []byte(`{"entryId":"entry_api","action":"block_source"}`), "application-block-key-0001")
	if blocked.Code != stdhttp.StatusOK {
		t.Fatalf("block Source status=%d body=%s", blocked.Code, blocked.Body.String())
	}
	blocks := do(stdhttp.MethodGet, "/api/tantan/v1/recommendation/blocks/sources", nil)
	if blocks.Code != stdhttp.StatusOK || !strings.Contains(blocks.Body.String(), `"sourceId":"feed_api"`) || !strings.Contains(blocks.Body.String(), `"name":"API Source"`) {
		t.Fatalf("list Source blocks status=%d body=%s", blocks.Code, blocks.Body.String())
	}
	restored := doWithKey(stdhttp.MethodDelete, "/api/tantan/v1/recommendation/blocks/sources/feed_api", nil, "application-restore-key-0001")
	if restored.Code != stdhttp.StatusOK {
		t.Fatalf("restore Source status=%d body=%s", restored.Code, restored.Body.String())
	}
	blocks = do(stdhttp.MethodGet, "/api/tantan/v1/recommendation/blocks/sources", nil)
	if blocks.Code != stdhttp.StatusOK || !strings.Contains(blocks.Body.String(), `"items":[]`) {
		t.Fatalf("Source block remained status=%d body=%s", blocks.Code, blocks.Body.String())
	}

	canary := "sk-tantan-api-contract-canary"
	invalidSettings := do(stdhttp.MethodPut, "/api/tantan/v1/settings/ai-provider", []byte(`{"providerId":"openai","model":"gpt-5-mini","apiKey":"`+canary+`","baseUrl":"https://attacker.invalid"}`))
	if invalidSettings.Code != stdhttp.StatusMethodNotAllowed && invalidSettings.Code != stdhttp.StatusNotFound || strings.Contains(invalidSettings.Body.String(), canary) {
		t.Fatalf("strict AI settings status=%d body=%s", invalidSettings.Code, invalidSettings.Body.String())
	}
	var databaseCanaryCount int
	if err := application.Store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_provider_configs_v1 WHERE provider_id LIKE ? OR model LIKE ?`, "%"+canary+"%", "%"+canary+"%").Scan(&databaseCanaryCount); err != nil || databaseCanaryCount != 0 {
		t.Fatalf("database canary count=%d err=%v", databaseCanaryCount, err)
	}
}
