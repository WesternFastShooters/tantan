package http_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/auth"
	"tantan.local/tantan-api/internal/folo"
	localhttp "tantan.local/tantan-api/internal/http"
	"tantan.local/tantan-api/internal/session"
)

type noOpFoloAuth struct{}

func (noOpFoloAuth) ApplyOneTimeToken(context.Context, string) (string, session.User, error) {
	return "upstream-token", session.User{ID: "user_1", Name: "Test"}, nil
}
func (noOpFoloAuth) SignOut(context.Context, string) error { return nil }

type routerSecrets struct{ values map[string]string }

func (store *routerSecrets) Get(_ context.Context, account string) (string, error) {
	return store.values[account], nil
}
func (store *routerSecrets) Set(_ context.Context, account, value string) error {
	store.values[account] = value
	return nil
}
func (store *routerSecrets) Delete(_ context.Context, account string) error {
	delete(store.values, account)
	return nil
}

func TestListenAddressRejectsNonLoopback(t *testing.T) {
	for _, address := range []string{"0.0.0.0:3000", "[::]:3000", "192.168.1.2:3000", "localhost:3000", "127.0.0.1:3001"} {
		if err := localhttp.ValidateListenAddr(address); err == nil {
			t.Errorf("accepted unsafe listen address %q", address)
		}
	}
	if err := localhttp.ValidateListenAddr("127.0.0.1:3000"); err != nil {
		t.Fatalf("rejected required loopback address: %v", err)
	}
}

func TestRouterReturnsRoutePolicyResultBeforeOriginCheck(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(stdhttp.HandlerFunc(func(stdhttp.ResponseWriter, *stdhttp.Request) {
		upstreamCalls++
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	policy, _ := folo.LoadPolicy()
	secrets := &routerSecrets{values: map[string]string{}}
	proxy, err := folo.NewProxy(folo.ProxyConfig{Policy: policy, Upstream: upstreamURL, Client: upstream.Client(), Secrets: secrets})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	router := localhttp.NewRouter(localhttp.RouterConfig{Proxy: proxy, Sessions: session.NewStore(time.Now)})

	for _, test := range []struct {
		path   string
		status int
	}{
		{path: "/ai/summary", status: stdhttp.StatusGone},
		{path: "/not-in-policy", status: stdhttp.StatusForbidden},
	} {
		request := httptest.NewRequest(stdhttp.MethodPost, "http://127.0.0.1:3000"+test.path, nil)
		request.Host = "127.0.0.1:3000"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("%s status=%d", test.path, response.Code)
		}
	}

	allowedPreflight := httptest.NewRequest(stdhttp.MethodOptions, "http://127.0.0.1:3000/reads", nil)
	allowedPreflight.Host = "127.0.0.1:3000"
	allowedPreflight.Header.Set("Origin", "http://127.0.0.1:5173")
	allowedPreflight.Header.Set("Access-Control-Request-Method", stdhttp.MethodPost)
	allowedPreflight.Header.Set("Access-Control-Request-Headers", "Content-Type")
	allowedPreflightResponse := httptest.NewRecorder()
	router.ServeHTTP(allowedPreflightResponse, allowedPreflight)
	if allowedPreflightResponse.Code != stdhttp.StatusNoContent || allowedPreflightResponse.Header().Get("Access-Control-Allow-Methods") != stdhttp.MethodPost {
		t.Fatalf("allowed preflight response=%d methods=%q", allowedPreflightResponse.Code, allowedPreflightResponse.Header().Get("Access-Control-Allow-Methods"))
	}

	for _, preflight := range []struct {
		path    string
		headers string
	}{
		{path: "/not-in-policy"},
		{path: "/reads", headers: "Authorization"},
	} {
		request := httptest.NewRequest(stdhttp.MethodOptions, "http://127.0.0.1:3000"+preflight.path, nil)
		request.Host = "127.0.0.1:3000"
		request.Header.Set("Origin", "http://127.0.0.1:5173")
		request.Header.Set("Access-Control-Request-Method", stdhttp.MethodPost)
		request.Header.Set("Access-Control-Request-Headers", preflight.headers)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != stdhttp.StatusForbidden {
			t.Fatalf("rejected preflight %s headers=%q status=%d", preflight.path, preflight.headers, response.Code)
		}
	}
	if upstreamCalls != 0 {
		t.Fatalf("denied requests made %d upstream calls", upstreamCalls)
	}
}

func TestLocalMuxAuthorizesOnlyRegisteredMethodAndPattern(t *testing.T) {
	mux := localhttp.NewLocalMux()
	mux.HandleFunc(stdhttp.MethodGet, "/tantan/v1/entries/{id}/enrichment", func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		writer.WriteHeader(stdhttp.StatusOK)
	})
	if !mux.AllowsPreflight(stdhttp.MethodGet, "/tantan/v1/entries/entry_1/enrichment") {
		t.Fatal("registered local route was not authorized")
	}
	if mux.AllowsPreflight(stdhttp.MethodPost, "/tantan/v1/entries/entry_1/enrichment") {
		t.Fatal("unregistered local method was authorized")
	}
	if mux.AllowsPreflight(stdhttp.MethodGet, "/tantan/v1/entries/entry_1/unknown") {
		t.Fatal("unregistered local path was authorized")
	}
}

func TestRouterDispatchesFutureLocalHandlersWithSessionContext(t *testing.T) {
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	sessions := session.NewStore(func() time.Time { return now })
	raw, err := session.NewToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	_, err = sessions.Create(context.Background(), raw, session.User{ID: "user_1", Name: "Test"}, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	local := stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		record, ok := session.FromContext(request.Context())
		if !ok || record.User.ID != "user_1" || record.Timezone != "Asia/Tokyo" {
			t.Fatalf("local session context=%#v ok=%v", record, ok)
		}
		if request.Header.Get("Cookie") != "" || request.Header.Get("Authorization") != "" || request.Header.Get("Referer") != "" {
			t.Fatalf("local handler received browser credentials: %#v", request.Header)
		}
		writer.WriteHeader(stdhttp.StatusAccepted)
	})
	router := localhttp.NewRouter(localhttp.RouterConfig{Sessions: sessions, Local: local})
	request := httptest.NewRequest(stdhttp.MethodGet, "http://127.0.0.1:3000/tantan/v1/topics", nil)
	request.Host = "127.0.0.1:3000"
	request.Header.Set("X-Tantan-Timezone", "Asia/Tokyo")
	request.Header.Set("Authorization", "Bearer browser-CANARY")
	request.Header.Set("Referer", "http://127.0.0.1:5173/search?q=private-CANARY")
	request.AddCookie(&stdhttp.Cookie{Name: session.LocalCookieName, Value: raw})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusAccepted {
		t.Fatalf("local handler status=%d", response.Code)
	}
}

func TestRouterRequiresSessionForEveryLocalAPI(t *testing.T) {
	localCalls := 0
	local := stdhttp.HandlerFunc(func(stdhttp.ResponseWriter, *stdhttp.Request) {
		localCalls++
	})
	router := localhttp.NewRouter(localhttp.RouterConfig{Sessions: session.NewStore(time.Now), Local: local})
	request := httptest.NewRequest(stdhttp.MethodGet, "http://127.0.0.1:3000/tantan/v1/topics", nil)
	request.Host = "127.0.0.1:3000"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusUnauthorized || localCalls != 0 {
		t.Fatalf("unauthenticated local response=%d calls=%d", response.Code, localCalls)
	}
}

func TestRouterAuthCallbackPreservesFlowButRedactsBrowserCredentials(t *testing.T) {
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	sessions := session.NewStore(func() time.Time { return now })
	secrets := &routerSecrets{values: map[string]string{}}
	logs := &bytes.Buffer{}
	bridge, err := auth.NewBridge(auth.Config{
		FoloWebURL:  "https://app.folo.is",
		CallbackURL: "http://127.0.0.1:3000/auth/folo/callback",
		Now:         func() time.Time { return now },
		Sessions:    sessions,
		Secrets:     secrets,
		Folo:        noOpFoloAuth{},
	})
	if err != nil {
		t.Fatalf("create bridge: %v", err)
	}
	router := localhttp.NewRouter(localhttp.RouterConfig{
		Auth:     bridge,
		Sessions: sessions,
		Logger:   slog.New(slog.NewJSONHandler(logs, nil)),
	})
	startRequest := httptest.NewRequest(stdhttp.MethodGet, "http://127.0.0.1:3000/auth/folo/start", nil)
	startRequest.Host = "127.0.0.1:3000"
	startResponse := httptest.NewRecorder()
	router.ServeHTTP(startResponse, startRequest)
	var flowCookie *stdhttp.Cookie
	for _, cookie := range startResponse.Result().Cookies() {
		if cookie.Name == auth.FlowCookieName {
			flowCookie = cookie
		}
	}
	if startResponse.Code != stdhttp.StatusFound || flowCookie == nil {
		t.Fatalf("start response=%d flow=%#v", startResponse.Code, flowCookie)
	}

	const oneTimeToken = "one-time-token-CANARY-router-123456"
	callbackRequest := httptest.NewRequest(stdhttp.MethodGet, "http://127.0.0.1:3000/auth/folo/callback?token="+oneTimeToken, nil)
	callbackRequest.Host = "127.0.0.1:3000"
	callbackRequest.AddCookie(flowCookie)
	callbackRequest.AddCookie(&stdhttp.Cookie{Name: "browser_secret", Value: "browser-cookie-CANARY"})
	callbackRequest.Header.Set("Authorization", "Bearer authorization-CANARY")
	callbackResponse := httptest.NewRecorder()
	router.ServeHTTP(callbackResponse, callbackRequest)

	if callbackResponse.Code != stdhttp.StatusFound || sessions.Len() != 1 || len(secrets.values) != 1 {
		t.Fatalf("callback response=%d sessions=%d secrets=%d", callbackResponse.Code, sessions.Len(), len(secrets.values))
	}
	for _, secret := range []string{oneTimeToken, "browser-cookie-CANARY", "authorization-CANARY", "upstream-token"} {
		responseHeaders := strings.Join(append(callbackResponse.Header().Values("Set-Cookie"), callbackResponse.Header().Get("Location")), "\n")
		if strings.Contains(logs.String(), secret) || strings.Contains(callbackResponse.Body.String(), secret) || strings.Contains(responseHeaders, secret) {
			t.Fatalf("router callback exposed %q", secret)
		}
	}
}

func TestRouterRejectsHostAndMutationOriginBeforeUpstream(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(stdhttp.HandlerFunc(func(stdhttp.ResponseWriter, *stdhttp.Request) {
		upstreamCalls++
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	policy, _ := folo.LoadPolicy()
	secrets := &routerSecrets{values: map[string]string{}}
	proxy, err := folo.NewProxy(folo.ProxyConfig{
		Policy:   policy,
		Upstream: upstreamURL,
		Client:   upstream.Client(),
		Secrets:  secrets,
		Logger:   slog.New(slog.NewJSONHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	sessions := session.NewStore(func() time.Time { return now })
	bridge, err := auth.NewBridge(auth.Config{
		FoloWebURL:  "https://app.folo.is",
		CallbackURL: "http://127.0.0.1:3000/auth/folo/callback",
		Now:         func() time.Time { return now },
		Logger:      slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Sessions:    sessions,
		Secrets:     secrets,
		Folo:        noOpFoloAuth{},
	})
	if err != nil {
		t.Fatalf("create bridge: %v", err)
	}
	logs := &bytes.Buffer{}
	router := localhttp.NewRouter(localhttp.RouterConfig{
		Auth:     bridge,
		Proxy:    proxy,
		Sessions: sessions,
		Logger:   slog.New(slog.NewJSONHandler(logs, nil)),
	})

	badHost := httptest.NewRequest(stdhttp.MethodGet, "http://attacker.invalid/entries", nil)
	badHost.Host = "127.0.0.1:3000.attacker.invalid"
	badHostResponse := httptest.NewRecorder()
	router.ServeHTTP(badHostResponse, badHost)
	if badHostResponse.Code != stdhttp.StatusForbidden {
		t.Fatalf("bad Host status = %d", badHostResponse.Code)
	}

	raw, _ := session.NewToken()
	record, err := sessions.Create(context.Background(), raw, session.User{ID: "user_1", Name: "Test"}, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	secrets.values[record.IDHash] = "upstream-token"
	badOrigin := httptest.NewRequest(stdhttp.MethodPost, "http://127.0.0.1:3000/reads", nil)
	badOrigin.Host = "127.0.0.1:3000"
	badOrigin.Header.Set("Origin", "https://origin-CANARY.attacker.invalid")
	badOrigin.AddCookie(&stdhttp.Cookie{Name: session.LocalCookieName, Value: raw})
	badOriginResponse := httptest.NewRecorder()
	router.ServeHTTP(badOriginResponse, badOrigin)
	if badOriginResponse.Code != stdhttp.StatusForbidden {
		t.Fatalf("bad Origin status = %d", badOriginResponse.Code)
	}
	if upstreamCalls != 0 {
		t.Fatalf("rejected requests made %d upstream calls", upstreamCalls)
	}
	if strings.Contains(logs.String(), "CANARY") || strings.Contains(logs.String(), "attacker.invalid") {
		t.Fatal("security logs contain attacker-controlled Host or Origin")
	}
}
