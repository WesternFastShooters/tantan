package folo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/folo"
	"tantan.local/tantan-api/internal/session"
)

var enabledRoutePaths = map[string]string{
	"auth-session":             "/better-auth/get-session",
	"auth-token-apply":         "/better-auth/one-time-token/apply",
	"auth-sign-out":            "/better-auth/sign-out",
	"categories":               "/categories",
	"collections":              "/collections",
	"discover":                 "/discover",
	"discover-rsshub":          "/discover/rsshub",
	"discover-rsshub-route":    "/discover/rsshub/route",
	"entries":                  "/entries",
	"entries-preview":          "/entries/preview",
	"entries-readability":      "/entries/readability",
	"entries-stream":           "/entries/stream",
	"entries-check-new":        "/entries/check-new",
	"entries-tags-query":       "/entries/tags/query",
	"entries-read-history":     "/entries/read-histories/entry_1",
	"entries-inbox":            "/entries/inbox",
	"feeds":                    "/feeds",
	"feeds-refresh":            "/feeds/refresh",
	"inboxes":                  "/inboxes",
	"inboxes-list":             "/inboxes/list",
	"lists":                    "/lists",
	"lists-list":               "/lists/list",
	"lists-feeds":              "/lists/feeds",
	"profiles":                 "/profiles",
	"profiles-batch":           "/profiles/batch",
	"reads":                    "/reads",
	"reads-all":                "/reads/all",
	"reads-total-count":        "/reads/total-count",
	"settings":                 "/settings",
	"settings-tab":             "/settings/appearance",
	"subscriptions":            "/subscriptions",
	"subscriptions-batch":      "/subscriptions/batch",
	"subscriptions-import":     "/subscriptions/import",
	"subscriptions-export":     "/subscriptions/export",
	"subscriptions-parse-opml": "/subscriptions/parse-opml",
}

type enabledPolicyDocument struct {
	Enabled []struct {
		ID      string   `json:"id"`
		Methods []string `json:"methods"`
	} `json:"enabled"`
}

func TestProxyPreservesEveryEnabledMethodPathAndPerformanceBudget(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve proxy test path")
	}
	policyContents, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "route-policy.json"))
	if err != nil {
		t.Fatalf("read route policy: %v", err)
	}
	var document enabledPolicyDocument
	if err := json.Unmarshal(policyContents, &document); err != nil {
		t.Fatalf("decode route policy: %v", err)
	}
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalls++
		if request.Method != http.MethodGet {
			contents, err := io.ReadAll(request.Body)
			if err != nil || string(contents) != `{}` {
				t.Fatalf("%s %s body=%q err=%v", request.Method, request.URL.Path, contents, err)
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(writer, `{"ok":true}`)
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	policy, err := folo.LoadPolicy()
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	proxy, err := folo.NewProxy(folo.ProxyConfig{
		Policy:   policy,
		Upstream: upstreamURL,
		Client:   upstream.Client(),
		Secrets:  &memorySecrets{values: map[string]string{"session-hash": "upstream-token"}},
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	var durations []time.Duration
	var expectedCalls int
	for _, route := range document.Enabled {
		path, ok := enabledRoutePaths[route.ID]
		if !ok {
			t.Fatalf("enabled route %q has no compatibility fixture", route.ID)
		}
		for _, method := range route.Methods {
			expectedCalls++
			var body io.Reader
			if method != http.MethodGet {
				body = strings.NewReader(`{}`)
			}
			request := httptest.NewRequest(method, "http://127.0.0.1:3000"+path, body)
			if method != http.MethodGet {
				request.Header.Set("Origin", "http://127.0.0.1:3000")
			}
			request = request.WithContext(session.WithRecord(request.Context(), session.Record{
				IDHash:    "session-hash",
				ExpiresAt: time.Now().Add(time.Hour),
			}))
			response := httptest.NewRecorder()
			startedAt := time.Now()
			proxy.ServeHTTP(response, request)
			durations = append(durations, time.Since(startedAt))
			if response.Code != http.StatusCreated || response.Header().Get("Content-Type") != "application/json" || response.Body.String() != `{"ok":true}` {
				t.Fatalf("%s %s response=%d type=%q body=%q", method, path, response.Code, response.Header().Get("Content-Type"), response.Body.String())
			}
		}
	}
	if upstreamCalls != expectedCalls {
		t.Fatalf("upstreamCalls=%d expected=%d", upstreamCalls, expectedCalls)
	}
	sort.Slice(durations, func(left, right int) bool { return durations[left] < durations[right] })
	p95 := durations[(len(durations)*95-1)/100]
	t.Logf("enabled method/path fixtures=%d proxy P95=%s", expectedCalls, p95)
	if p95 > 30*time.Millisecond {
		t.Fatalf("proxy P95 overhead=%s exceeds 30ms budget", p95)
	}
}

type memorySecrets struct {
	mu     sync.Mutex
	values map[string]string
}

func (store *memorySecrets) Get(_ context.Context, account string) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.values[account], nil
}

func (store *memorySecrets) Set(_ context.Context, account, value string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.values[account] = value
	return nil
}

func (store *memorySecrets) Delete(_ context.Context, account string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.values, account)
	return nil
}

func TestProxyPreservesAllowedResponseAndControlsHeaders(t *testing.T) {
	const upstreamToken = "folo-token-CANARY-proxy"
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalls++
		if got := request.Header.Get("Cookie"); got != "__Secure-better-auth.session_token="+upstreamToken {
			t.Errorf("upstream cookie = %q", got)
		}
		for _, forbidden := range []string{"Authorization", "Forwarded", "X-Forwarded-For", "Proxy-Authorization", "Referer", "Te", "Trailer", "X-Real-Ip"} {
			if value := request.Header.Get(forbidden); value != "" {
				t.Errorf("upstream received %s=%q", forbidden, value)
			}
		}
		if value := request.Header.Get("X-Hop-Secret"); value != "" {
			t.Errorf("upstream received Connection-scoped header %q", value)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Access-Control-Allow-Origin", "https://attacker.invalid")
		writer.Header().Set("Connection", "X-Upstream-Hop")
		writer.Header().Set("X-Upstream-Hop", "must-not-reach-browser")
		writer.Header().Set("Set-Cookie", "__Secure-better-auth.session_token=must-not-reach-browser")
		writer.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(writer, `{"ok":true}`)
	}))
	defer upstream.Close()

	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream: %v", err)
	}
	policy, err := folo.LoadPolicy()
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	logs := &bytes.Buffer{}
	secrets := &memorySecrets{values: map[string]string{"session-hash": upstreamToken}}
	proxy, err := folo.NewProxy(folo.ProxyConfig{
		Policy:   policy,
		Upstream: upstreamURL,
		Client:   upstream.Client(),
		Secrets:  secrets,
		Logger:   slog.New(slog.NewJSONHandler(logs, nil)),
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3000/entries?limit=20", nil)
	request.Header.Set("Authorization", "browser-secret")
	request.Header.Set("Cookie", "browser-cookie=secret")
	request.Header.Set("Forwarded", "for=attacker.invalid")
	request.Header.Set("Referer", "http://127.0.0.1:5173/search?q=private-CANARY")
	request.Header.Set("Te", "trailers")
	request.Header.Set("X-Real-Ip", "198.51.100.1")
	request.Header.Set("Connection", "X-Hop-Secret")
	request.Header.Set("X-Hop-Secret", "connection-header-CANARY")
	request = request.WithContext(session.WithRecord(request.Context(), session.Record{
		IDHash:    "session-hash",
		User:      session.User{ID: "user_1", Name: "Test"},
		ExpiresAt: time.Now().Add(time.Hour),
	}))
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || response.Body.String() != `{"ok":true}` {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q", got)
	}
	if got := response.Header().Get("Set-Cookie"); got != "" {
		t.Fatalf("upstream Set-Cookie reached browser: %q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("upstream CORS header reached browser: %q", got)
	}
	if got := response.Header().Get("X-Upstream-Hop"); got != "" {
		t.Fatalf("upstream Connection-scoped header reached browser: %q", got)
	}
	if upstreamCalls != 1 {
		t.Fatalf("upstream calls = %d", upstreamCalls)
	}
	for _, secret := range []string{upstreamToken, "browser-secret", "browser-cookie", "connection-header-CANARY", "private-CANARY"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("logs contain secret %q", secret)
		}
	}
}

func FuzzRoutePolicyNeverBypassesDecisionClasses(fuzz *testing.F) {
	for _, seed := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/entries"},
		{method: http.MethodPost, path: "/%61i/summary"},
		{method: http.MethodPost, path: "/%2561i/summary"},
		{method: http.MethodGet, path: "/../entries"},
		{method: http.MethodGet, path: "/entries\\evil"},
		{method: "G\x00ET", path: "/entries"},
	} {
		fuzz.Add(seed.method, seed.path)
	}
	policy, err := folo.LoadPolicy()
	if err != nil {
		fuzz.Fatalf("load policy: %v", err)
	}
	fuzz.Fuzz(func(t *testing.T, method, path string) {
		decision := policy.Decide(method, path)
		switch decision.Kind {
		case folo.DecisionAllow:
			if decision.RouteID == "" || decision.Status != 0 || decision.MaxRequestBytes <= 0 || decision.MaxResponseBytes <= 0 {
				t.Fatalf("invalid allow decision: %#v", decision)
			}
		case folo.DecisionRemoved:
			if decision.Status != http.StatusGone || decision.Code != "FOLO_FEATURE_REMOVED" {
				t.Fatalf("invalid removed decision: %#v", decision)
			}
		case folo.DecisionDenied:
			if decision.Status != http.StatusForbidden || decision.Code != "FOLO_ROUTE_DENIED" {
				t.Fatalf("invalid deny decision: %#v", decision)
			}
		default:
			t.Fatalf("unknown decision kind: %#v", decision)
		}
	})
}

func TestDeniedAndRemovedRoutesNeverReachUpstream(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		upstreamCalls++
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	policy, _ := folo.LoadPolicy()
	proxy, err := folo.NewProxy(folo.ProxyConfig{
		Policy:   policy,
		Upstream: upstreamURL,
		Client:   upstream.Client(),
		Secrets:  &memorySecrets{values: map[string]string{}},
		Logger:   slog.New(slog.NewJSONHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	tests := []struct {
		method string
		path   string
		status int
		code   string
	}{
		{method: http.MethodPost, path: "/ai/summary", status: http.StatusGone, code: "FOLO_FEATURE_REMOVED"},
		{method: http.MethodGet, path: "/wallets/balance", status: http.StatusGone, code: "FOLO_FEATURE_REMOVED"},
		{method: http.MethodPost, path: "/payments/create", status: http.StatusGone, code: "FOLO_FEATURE_REMOVED"},
		{method: http.MethodGet, path: "/better-auth/stripe/list", status: http.StatusGone, code: "FOLO_FEATURE_REMOVED"},
		{method: http.MethodGet, path: "/referrals/list", status: http.StatusGone, code: "FOLO_FEATURE_REMOVED"},
		{method: http.MethodGet, path: "/trending/topics", status: http.StatusGone, code: "FOLO_FEATURE_REMOVED"},
		{method: http.MethodPost, path: "/rsshub/use", status: http.StatusGone, code: "FOLO_FEATURE_REMOVED"},
		{method: http.MethodPatch, path: "/settings/ai", status: http.StatusForbidden, code: "FOLO_ROUTE_DENIED"},
		{method: http.MethodGet, path: "/not-in-policy", status: http.StatusForbidden, code: "FOLO_ROUTE_DENIED"},
	}

	for _, test := range tests {
		request := httptest.NewRequest(test.method, "http://127.0.0.1:3000"+test.path, nil)
		response := httptest.NewRecorder()
		proxy.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("%s %s status = %d", test.method, test.path, response.Code)
		}
		var body struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if body.Error.Code != test.code {
			t.Fatalf("error code = %q", body.Error.Code)
		}
	}
	if upstreamCalls != 0 {
		t.Fatalf("denied routes made %d upstream calls", upstreamCalls)
	}
}

func TestProxyDoesNotFollowUpstreamRedirects(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalls++
		if request.URL.Path == "/entries" {
			http.Redirect(writer, request, "/must-not-follow", http.StatusFound)
			return
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	policy, _ := folo.LoadPolicy()
	secrets := &memorySecrets{values: map[string]string{"session-hash": "upstream-token"}}
	proxy, err := folo.NewProxy(folo.ProxyConfig{
		Policy:   policy,
		Upstream: upstreamURL,
		Client:   upstream.Client(),
		Secrets:  secrets,
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3000/entries", nil)
	request = request.WithContext(session.WithRecord(request.Context(), session.Record{IDHash: "session-hash", ExpiresAt: time.Now().Add(time.Hour)}))
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, request)

	if response.Code != http.StatusFound || upstreamCalls != 1 {
		t.Fatalf("redirect response=%d upstreamCalls=%d", response.Code, upstreamCalls)
	}
}

func TestDeniedRouteDoesNotLogAttackerControlledPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("denied request reached upstream")
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	policy, _ := folo.LoadPolicy()
	logs := &bytes.Buffer{}
	proxy, err := folo.NewProxy(folo.ProxyConfig{
		Policy:   policy,
		Upstream: upstreamURL,
		Client:   upstream.Client(),
		Secrets:  &memorySecrets{values: map[string]string{}},
		Logger:   slog.New(slog.NewJSONHandler(logs, nil)),
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3000/private-CANARY-path-secret", nil)
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d", response.Code)
	}
	if strings.Contains(logs.String(), "CANARY-path-secret") {
		t.Fatal("denied route logged attacker-controlled path")
	}
}

func TestProxyRejectsHeaderSmugglingWithoutDialOrLogLeak(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		upstreamCalls++
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	policy, _ := folo.LoadPolicy()
	logs := &bytes.Buffer{}
	proxy, err := folo.NewProxy(folo.ProxyConfig{
		Policy:   policy,
		Upstream: upstreamURL,
		Client:   upstream.Client(),
		Secrets:  &memorySecrets{values: map[string]string{"session-hash": "upstream-token"}},
		Logger:   slog.New(slog.NewJSONHandler(logs, nil)),
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3000/entries", nil)
	request.Header["X-Smuggle"] = []string{"value\r\nCookie: header-CANARY-secret"}
	request = request.WithContext(session.WithRecord(request.Context(), session.Record{
		IDHash:    "session-hash",
		ExpiresAt: time.Now().Add(time.Hour),
	}))
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway || upstreamCalls != 0 {
		t.Fatalf("smuggling response=%d upstreamCalls=%d", response.Code, upstreamCalls)
	}
	if strings.Contains(response.Body.String(), "CANARY") || strings.Contains(logs.String(), "CANARY") {
		t.Fatal("header smuggling canary reached response or logs")
	}
}

func TestProxyRejectsCorruptKeychainCookieWithoutDial(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		upstreamCalls++
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	policy, _ := folo.LoadPolicy()
	proxy, err := folo.NewProxy(folo.ProxyConfig{
		Policy:   policy,
		Upstream: upstreamURL,
		Client:   upstream.Client(),
		Secrets:  &memorySecrets{values: map[string]string{"session-hash": "unsafe;cookie"}},
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3000/entries", nil)
	request = request.WithContext(session.WithRecord(request.Context(), session.Record{IDHash: "session-hash"}))
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || upstreamCalls != 0 || strings.Contains(response.Body.String(), "unsafe") {
		t.Fatalf("corrupt cookie response=%d upstreamCalls=%d body=%q", response.Code, upstreamCalls, response.Body.String())
	}
}
