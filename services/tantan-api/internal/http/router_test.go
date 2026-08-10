package http_test

import (
	"bytes"
	"context"
	"encoding/json"
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

const publicOrigin = "https://reader.example.com"

type noOpFoloAuth struct{}

func (noOpFoloAuth) ApplyOneTimeToken(context.Context, string) (string, session.User, error) {
	return "upstream-token", session.User{ID: "user_1", Name: "Test"}, nil
}
func (noOpFoloAuth) SignInEmail(context.Context, string, string) (folo.LoginResult, error) {
	return folo.LoginResult{SessionToken: "upstream-token", User: session.User{ID: "user_1", Name: "Test"}}, nil
}
func (noOpFoloAuth) VerifyTOTP(context.Context, string, string) (folo.LoginResult, error) {
	return folo.LoginResult{SessionToken: "upstream-token", User: session.User{ID: "user_1", Name: "Test"}}, nil
}
func (noOpFoloAuth) SignOut(context.Context, string) error { return nil }

type replayStore struct{ reserved map[string]bool }

func (store *replayStore) Reserve(_ context.Context, hash string, _ time.Time) (bool, error) {
	if store.reserved[hash] {
		return false, nil
	}
	store.reserved[hash] = true
	return true, nil
}

type routerOwnerStore struct{ user session.User }

func (store routerOwnerStore) FindOwner(context.Context) (session.User, bool, error) {
	return store.user, store.user.ID != "", nil
}
func (store *replayStore) Release(_ context.Context, hash string) error {
	delete(store.reserved, hash)
	return nil
}

type routerSecrets struct{ values map[string]string }

func (store *routerSecrets) Get(_ context.Context, account string) (string, error) {
	value, ok := store.values[account]
	if !ok {
		return "", session.ErrSecretNotFound
	}
	return value, nil
}
func (store *routerSecrets) Set(_ context.Context, account, value string) error {
	store.values[account] = value
	return nil
}
func (store *routerSecrets) Delete(_ context.Context, account string) error {
	delete(store.values, account)
	return nil
}

func newRouter(t *testing.T, config localhttp.RouterConfig) stdhttp.Handler {
	t.Helper()
	if config.PublicOrigin == "" {
		config.PublicOrigin = publicOrigin
	}
	router, err := localhttp.NewRouter(config)
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	return router
}

func createSession(t *testing.T, sessions *session.Store, secrets *routerSecrets, now time.Time) (string, string, session.Record) {
	t.Helper()
	raw, err := session.NewToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	csrf, err := session.NewCSRFToken()
	if err != nil {
		t.Fatalf("new csrf: %v", err)
	}
	record, err := sessions.CreateWithCSRF(context.Background(), raw, csrf, session.User{ID: "user_1", Name: "Test"}, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if secrets != nil {
		secrets.values[record.SecretRef] = "upstream-token"
	}
	return raw, csrf, record
}

func addSessionRequestHeaders(request *stdhttp.Request, raw, csrf string) {
	request.Host = "reader.example.com"
	request.Header.Set("Origin", publicOrigin)
	request.Header.Set("X-CSRF-Token", csrf)
	request.AddCookie(&stdhttp.Cookie{Name: session.LocalCookieName, Value: raw})
}

func TestListenAndPublicOriginConfigurationFailClosed(t *testing.T) {
	for _, address := range []string{"0.0.0.0:3000", "[::]:3000", "192.168.1.2:3000", "localhost:3000", "127.0.0.1:3001"} {
		if err := localhttp.ValidateListenAddr(address); err == nil {
			t.Errorf("accepted unsafe listen address %q", address)
		}
	}
	if err := localhttp.ValidateListenAddr("127.0.0.1:3000"); err != nil {
		t.Fatalf("rejected loopback address: %v", err)
	}
	if err := localhttp.ValidateRuntimeListenAddr("0.0.0.0:8080", true); err != nil {
		t.Fatalf("rejected isolated Cloudflare container address: %v", err)
	}
	if err := localhttp.ValidateRuntimeListenAddr("0.0.0.0:8080", false); err == nil {
		t.Fatal("accepted public bind outside isolated container mode")
	}
	if err := localhttp.ValidateRuntimeListenAddr("127.0.0.1:3000", true); err == nil {
		t.Fatal("accepted loopback bind for Cloudflare container mode")
	}
	for _, config := range []localhttp.RouterConfig{
		{PublicOrigin: "http://public.example.com"},
		{PublicOrigin: "https://reader.example.com/path"},
		{PublicOrigin: publicOrigin, TrustedProxyCIDRs: []string{"not-a-cidr"}},
	} {
		if _, err := localhttp.NewRouter(config); err == nil {
			t.Fatalf("accepted unsafe router config %#v", config)
		}
	}
}

func TestRouterLocalAPIRequiresSessionOriginAndCSRF(t *testing.T) {
	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	sessions := session.NewStore(func() time.Time { return now })
	raw, csrf, _ := createSession(t, sessions, nil, now)
	localCalls := 0
	local := stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		localCalls++
		record, ok := session.FromContext(request.Context())
		if !ok || record.User.ID != "user_1" || request.URL.Path != "/tantan/v1/topics" {
			t.Fatalf("local request path=%q session=%#v ok=%v", request.URL.Path, record, ok)
		}
		if request.Header.Get("Cookie") != "" || request.Header.Get("Authorization") != "" || request.Header.Get("Referer") != "" {
			t.Fatalf("local handler received browser credentials: %#v", request.Header)
		}
		writer.WriteHeader(stdhttp.StatusNoContent)
	})
	router := newRouter(t, localhttp.RouterConfig{Sessions: sessions, Local: local})

	unauthenticated := httptest.NewRequest(stdhttp.MethodGet, publicOrigin+"/api/tantan/v1/topics", nil)
	unauthenticated.Host = "reader.example.com"
	unauthenticatedResponse := httptest.NewRecorder()
	router.ServeHTTP(unauthenticatedResponse, unauthenticated)
	if unauthenticatedResponse.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", unauthenticatedResponse.Code)
	}

	valid := httptest.NewRequest(stdhttp.MethodPatch, publicOrigin+"/api/tantan/v1/topics", strings.NewReader(`{}`))
	addSessionRequestHeaders(valid, raw, csrf)
	valid.Header.Set("Authorization", "Bearer browser-CANARY")
	valid.Header.Set("Referer", publicOrigin+"/private-CANARY")
	validResponse := httptest.NewRecorder()
	router.ServeHTTP(validResponse, valid)
	if validResponse.Code != stdhttp.StatusNoContent || localCalls != 1 {
		t.Fatalf("valid response=%d calls=%d", validResponse.Code, localCalls)
	}

	for _, mutate := range []func(*stdhttp.Request){
		func(request *stdhttp.Request) { request.Header.Del("X-CSRF-Token") },
		func(request *stdhttp.Request) { request.Header.Set("X-CSRF-Token", "wrong-token") },
		func(request *stdhttp.Request) { request.Header.Del("Origin") },
	} {
		request := httptest.NewRequest(stdhttp.MethodPatch, publicOrigin+"/api/tantan/v1/topics", strings.NewReader(`{}`))
		addSessionRequestHeaders(request, raw, csrf)
		mutate(request)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != stdhttp.StatusForbidden {
			t.Fatalf("unsafe mutation status=%d body=%s", response.Code, response.Body.String())
		}
	}
	if localCalls != 1 {
		t.Fatalf("unsafe mutations reached local handler: %d", localCalls)
	}
}

func TestRouterValidatesHostOriginAndTrustedProxyBeforeDispatch(t *testing.T) {
	staticCalls := 0
	logs := &bytes.Buffer{}
	router := newRouter(t, localhttp.RouterConfig{
		TrustedProxyCIDRs: []string{"10.0.0.0/8"},
		Static: stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
			staticCalls++
			writer.WriteHeader(stdhttp.StatusNoContent)
		}),
		Logger: slog.New(slog.NewJSONHandler(logs, nil)),
	})

	for _, request := range []*stdhttp.Request{
		hostRequest("reader.example.com.attacker.invalid"),
		originRequest("https://origin-CANARY.attacker.invalid"),
		forwardedRequest("192.0.2.10:1234", "reader.example.com", "https"),
		forwardedRequest("10.2.3.4:1234", "attacker.invalid", "https"),
		forwardedRequest("10.2.3.4:1234", "reader.example.com", "http"),
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != stdhttp.StatusForbidden {
			t.Fatalf("unsafe authority status=%d", response.Code)
		}
	}

	trusted := forwardedRequest("10.2.3.4:1234", "reader.example.com", "https")
	trustedResponse := httptest.NewRecorder()
	router.ServeHTTP(trustedResponse, trusted)
	if trustedResponse.Code != stdhttp.StatusNoContent || staticCalls != 1 {
		t.Fatalf("trusted proxy response=%d calls=%d", trustedResponse.Code, staticCalls)
	}
	if strings.Contains(logs.String(), "CANARY") || strings.Contains(logs.String(), "attacker.invalid") {
		t.Fatal("security log contains attacker-controlled authority")
	}
}

func TestSingleUserSessionRequiresHeaderInjectedByTrustedProxy(t *testing.T) {
	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	sessions := session.NewStore(func() time.Time { return now })
	secrets := &routerSecrets{values: map[string]string{auth.SingleUserSecretRef: "upstream-token"}}
	bridge, err := auth.NewBridge(auth.Config{
		PublicOrigin: publicOrigin,
		FoloWebURL:   "https://app.folo.is",
		Now:          func() time.Time { return now },
		Sessions:     sessions,
		Secrets:      secrets,
		Replays:      &replayStore{reserved: map[string]bool{}},
		Folo:         noOpFoloAuth{},
		SingleUser:   true,
		Owners:       routerOwnerStore{user: session.User{ID: "owner_1", Name: "Owner"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	router := newRouter(t, localhttp.RouterConfig{
		TrustedProxyCIDRs:  []string{"10.0.0.0/8"},
		SingleUserAccessID: "admin",
		Auth:               bridge,
		Sessions:           sessions,
	})

	for _, request := range []*stdhttp.Request{
		forwardedRequest("192.0.2.10:1234", "reader.example.com", "https"),
		forwardedRequest("10.2.3.4:1234", "reader.example.com", "https"),
	} {
		request.URL.Path = "/api/tantan/v1/session"
		request.Header.Set("X-Tantan-Authenticated-Owner", "admin")
		if strings.HasPrefix(request.RemoteAddr, "10.") {
			request.Header.Del("X-Tantan-Authenticated-Owner")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code == stdhttp.StatusOK {
			t.Fatalf("untrusted request received a session: remote=%s", request.RemoteAddr)
		}
	}

	trusted := forwardedRequest("10.2.3.4:1234", "reader.example.com", "https")
	trusted.URL.Path = "/api/tantan/v1/session"
	trusted.Header.Set("X-Tantan-Authenticated-Owner", "admin")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, trusted)
	if response.Code != stdhttp.StatusOK || sessions.Len() != 1 {
		t.Fatalf("trusted status=%d sessions=%d body=%s", response.Code, sessions.Len(), response.Body.String())
	}
}

func TestSingleUserSessionAcceptsOnlyExactCloudflareGatewaySecret(t *testing.T) {
	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	sessions := session.NewStore(func() time.Time { return now })
	secrets := &routerSecrets{values: map[string]string{auth.SingleUserSecretRef: "upstream-token"}}
	bridge, err := auth.NewBridge(auth.Config{
		PublicOrigin: publicOrigin,
		FoloWebURL:   "https://app.folo.is",
		Now:          func() time.Time { return now },
		Sessions:     sessions,
		Secrets:      secrets,
		Replays:      &replayStore{reserved: map[string]bool{}},
		Folo:         noOpFoloAuth{},
		SingleUser:   true,
		Owners:       routerOwnerStore{user: session.User{ID: "owner_1", Name: "Owner"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	const gatewaySecret = "0123456789abcdef0123456789abcdef"
	router := newRouter(t, localhttp.RouterConfig{
		SingleUserAccessID: "admin",
		GatewaySecret:      gatewaySecret,
		Auth:               bridge,
		Sessions:           sessions,
	})

	for _, supplied := range []string{"", "wrong", gatewaySecret + "x"} {
		request := hostRequest("reader.example.com")
		request.URL.Path = "/api/tantan/v1/session"
		request.Header.Set("X-Tantan-Authenticated-Owner", "admin")
		if supplied != "" {
			request.Header.Set("X-Tantan-Gateway-Secret", supplied)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code == stdhttp.StatusOK {
			t.Fatalf("invalid gateway secret received a session: length=%d", len(supplied))
		}
	}

	request := hostRequest("reader.example.com")
	request.URL.Path = "/api/tantan/v1/session"
	request.Header.Set("X-Tantan-Authenticated-Owner", "admin")
	request.Header.Set("X-Tantan-Gateway-Secret", gatewaySecret)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusOK || sessions.Len() != 1 {
		t.Fatalf("Cloudflare gateway status=%d sessions=%d body=%s", response.Code, sessions.Len(), response.Body.String())
	}
}

func TestRouterFoloPrefixDefaultDenyAndMutationCSRF(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		upstreamCalls++
		if request.URL.Path != "/reads" || request.Header.Get("Authorization") != "" || !strings.Contains(request.Header.Get("Cookie"), "upstream-token") {
			t.Fatalf("unsafe upstream request path=%q headers=%#v", request.URL.Path, request.Header)
		}
		writer.WriteHeader(stdhttp.StatusNoContent)
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	policy, err := folo.LoadPolicy()
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	secrets := &routerSecrets{values: map[string]string{}}
	proxy, err := folo.NewProxy(folo.ProxyConfig{Policy: policy, Upstream: upstreamURL, PublicOrigin: publicOrigin, Client: upstream.Client(), Secrets: secrets})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	sessions := session.NewStore(func() time.Time { return now })
	raw, csrf, _ := createSession(t, sessions, secrets, now)
	router := newRouter(t, localhttp.RouterConfig{Proxy: proxy, Sessions: sessions})

	for _, path := range []string{"/api/folo/not-in-policy", "/api/folo/ai/summary"} {
		request := httptest.NewRequest(stdhttp.MethodPost, publicOrigin+path, nil)
		request.Host = "reader.example.com"
		request.Header.Set("Origin", publicOrigin)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != stdhttp.StatusForbidden && response.Code != stdhttp.StatusGone {
			t.Fatalf("denied path %s status=%d", path, response.Code)
		}
	}

	badCSRF := httptest.NewRequest(stdhttp.MethodPost, publicOrigin+"/api/folo/reads", nil)
	addSessionRequestHeaders(badCSRF, raw, "wrong-token")
	badCSRFResponse := httptest.NewRecorder()
	router.ServeHTTP(badCSRFResponse, badCSRF)
	if badCSRFResponse.Code != stdhttp.StatusForbidden || upstreamCalls != 0 {
		t.Fatalf("bad csrf response=%d calls=%d", badCSRFResponse.Code, upstreamCalls)
	}

	valid := httptest.NewRequest(stdhttp.MethodPost, publicOrigin+"/api/folo/reads", nil)
	addSessionRequestHeaders(valid, raw, csrf)
	valid.Header.Set("Authorization", "Bearer browser-CANARY")
	validResponse := httptest.NewRecorder()
	router.ServeHTTP(validResponse, valid)
	if validResponse.Code != stdhttp.StatusNoContent || upstreamCalls != 1 {
		t.Fatalf("valid proxy response=%d calls=%d", validResponse.Code, upstreamCalls)
	}
}

func TestRouterDispatchesAuthHealthAndStaticRoutes(t *testing.T) {
	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	sessions := session.NewStore(func() time.Time { return now })
	secrets := &routerSecrets{values: map[string]string{}}
	bridge, err := auth.NewBridge(auth.Config{
		PublicOrigin: publicOrigin,
		FoloWebURL:   "https://app.folo.is",
		Now:          func() time.Time { return now },
		Sessions:     sessions,
		Secrets:      secrets,
		Replays:      &replayStore{reserved: map[string]bool{}},
		Folo:         noOpFoloAuth{},
	})
	if err != nil {
		t.Fatalf("create auth bridge: %v", err)
	}
	staticCalls := 0
	router := newRouter(t, localhttp.RouterConfig{
		Auth:     bridge,
		Sessions: sessions,
		Static: stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
			staticCalls++
			_, _ = io.WriteString(writer, "mobile shell")
		}),
	})

	providers := httptest.NewRequest(stdhttp.MethodGet, publicOrigin+"/api/auth/folo/providers", nil)
	providers.Host = "reader.example.com"
	providersResponse := httptest.NewRecorder()
	router.ServeHTTP(providersResponse, providers)
	var providersBody map[string]any
	if providersResponse.Code != stdhttp.StatusOK || json.Unmarshal(providersResponse.Body.Bytes(), &providersBody) != nil {
		t.Fatalf("providers response=%d body=%s", providersResponse.Code, providersResponse.Body.String())
	}

	health := httptest.NewRequest(stdhttp.MethodGet, publicOrigin+"/api/healthz", nil)
	health.Host = "reader.example.com"
	healthResponse := httptest.NewRecorder()
	router.ServeHTTP(healthResponse, health)
	if healthResponse.Code != stdhttp.StatusOK {
		t.Fatalf("health status=%d", healthResponse.Code)
	}

	staticRequest := httptest.NewRequest(stdhttp.MethodGet, publicOrigin+"/settings", nil)
	staticRequest.Host = "reader.example.com"
	staticResponse := httptest.NewRecorder()
	router.ServeHTTP(staticResponse, staticRequest)
	if staticResponse.Code != stdhttp.StatusOK || staticResponse.Body.String() != "mobile shell" || staticCalls != 1 {
		t.Fatalf("static response=%d body=%q calls=%d", staticResponse.Code, staticResponse.Body.String(), staticCalls)
	}
}

func hostRequest(host string) *stdhttp.Request {
	request := httptest.NewRequest(stdhttp.MethodGet, publicOrigin+"/", nil)
	request.Host = host
	return request
}

func originRequest(origin string) *stdhttp.Request {
	request := hostRequest("reader.example.com")
	request.Header.Set("Origin", origin)
	return request
}

func forwardedRequest(remoteAddress, host, protocol string) *stdhttp.Request {
	request := httptest.NewRequest(stdhttp.MethodGet, "http://internal:3000/", nil)
	request.Host = "internal:3000"
	request.RemoteAddr = remoteAddress
	request.Header.Set("X-Forwarded-For", "203.0.113.8")
	request.Header.Set("X-Forwarded-Host", host)
	request.Header.Set("X-Forwarded-Proto", protocol)
	return request
}
