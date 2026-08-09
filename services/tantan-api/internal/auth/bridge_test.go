package auth_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/auth"
	"tantan.local/tantan-api/internal/session"
)

type fakeFoloAuth struct {
	applyCalls     int
	signOutCalls   int
	signedOutToken string
	token          string
	user           session.User
}

func (fake *fakeFoloAuth) ApplyOneTimeToken(_ context.Context, _ string) (string, session.User, error) {
	fake.applyCalls++
	return fake.token, fake.user, nil
}

func (fake *fakeFoloAuth) SignOut(_ context.Context, token string) error {
	fake.signOutCalls++
	fake.signedOutToken = token
	return nil
}

type fakeSecrets struct {
	mu       sync.Mutex
	values   map[string]string
	setError error
}

type failingSessionBackend struct{}

func (failingSessionBackend) SaveSession(context.Context, session.Record) error {
	return errors.New("session database unavailable")
}

func (failingSessionBackend) FindSession(context.Context, string) (session.Record, bool, error) {
	return session.Record{}, false, errors.New("session database unavailable")
}

func (failingSessionBackend) DeleteSession(context.Context, string) error {
	return errors.New("session database unavailable")
}

func (store *fakeSecrets) Get(_ context.Context, account string) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.values[account], nil
}

func (store *fakeSecrets) Set(_ context.Context, account, value string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.setError != nil {
		return store.setError
	}
	store.values[account] = value
	return nil
}

func (store *fakeSecrets) Delete(_ context.Context, account string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.values, account)
	return nil
}

func newBridge(t *testing.T, secretStore *fakeSecrets, logs *bytes.Buffer) (*auth.Bridge, *fakeFoloAuth, *session.Store) {
	t.Helper()
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	foloAuth := &fakeFoloAuth{
		token: "folo-session-CANARY-auth",
		user:  session.User{ID: "user_1", Name: "Test User"},
	}
	sessions := session.NewStore(func() time.Time { return now })
	bridge, err := auth.NewBridge(auth.Config{
		FoloWebURL:  "https://app.folo.is",
		CallbackURL: "http://127.0.0.1:3000/auth/folo/callback",
		Now:         func() time.Time { return now },
		Logger:      slog.New(slog.NewJSONHandler(logs, nil)),
		Sessions:    sessions,
		Secrets:     secretStore,
		Folo:        foloAuth,
	})
	if err != nil {
		t.Fatalf("create bridge: %v", err)
	}
	return bridge, foloAuth, sessions
}

func startFlow(t *testing.T, bridge *auth.Bridge) *http.Cookie {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3000/auth/folo/start", nil)
	request.Host = "127.0.0.1:3000"
	response := httptest.NewRecorder()
	bridge.Start(response, request)
	if response.Code != http.StatusFound {
		t.Fatalf("start status = %d body=%s", response.Code, response.Body.String())
	}
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	if location.Scheme != "https" || location.Host != "app.folo.is" {
		t.Fatalf("unexpected auth redirect: %s", location)
	}
	if callback := location.Query().Get("cli_callback"); callback != "http://127.0.0.1:3000/auth/folo/callback" {
		t.Fatalf("callback URL = %q", callback)
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == auth.FlowCookieName {
			if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
				t.Fatalf("unsafe flow cookie: %#v", cookie)
			}
			return cookie
		}
	}
	t.Fatal("flow cookie missing")
	return nil
}

func TestAuthCallbackStoresFoloTokenOnlyInSecretStore(t *testing.T) {
	logs := &bytes.Buffer{}
	secrets := &fakeSecrets{values: map[string]string{}}
	bridge, foloAuth, sessions := newBridge(t, secrets, logs)
	flowCookie := startFlow(t, bridge)

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3000/auth/folo/callback?token=one-time-token-1234567890", nil)
	request.Host = "127.0.0.1:3000"
	request.AddCookie(flowCookie)
	response := httptest.NewRecorder()
	bridge.Callback(response, request)

	if response.Code != http.StatusFound || response.Header().Get("Location") != "/" {
		t.Fatalf("callback response = %d location=%q body=%q", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	if foloAuth.applyCalls != 1 {
		t.Fatalf("apply calls = %d", foloAuth.applyCalls)
	}
	var localCookie *http.Cookie
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == session.LocalCookieName {
			localCookie = cookie
		}
	}
	if localCookie == nil || !localCookie.HttpOnly || localCookie.Value == "" {
		t.Fatalf("local session cookie missing or unsafe: %#v", localCookie)
	}
	canary := foloAuth.token
	if strings.Contains(response.Header().Get("Set-Cookie"), canary) || strings.Contains(response.Body.String(), canary) {
		t.Fatal("Folo token reached browser response")
	}
	record, ok, err := sessions.LookupRaw(context.Background(), localCookie.Value)
	if err != nil {
		t.Fatalf("lookup local session: %v", err)
	}
	if !ok {
		t.Fatal("local session not persisted")
	}
	if got := secrets.values[record.IDHash]; got != canary {
		t.Fatalf("secret store token = %q", got)
	}
	if strings.Contains(logs.String(), canary) || strings.Contains(logs.String(), "one-time-token-1234567890") {
		t.Fatal("auth logs contain token")
	}

	replay := httptest.NewRecorder()
	bridge.Callback(replay, request)
	if replay.Code != http.StatusBadRequest {
		t.Fatalf("replayed flow status = %d", replay.Code)
	}

	freshFlow := startFlow(t, bridge)
	freshReplayRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3000/auth/folo/callback?token=one-time-token-1234567890", nil)
	freshReplayRequest.Host = "127.0.0.1:3000"
	freshReplayRequest.AddCookie(freshFlow)
	freshReplay := httptest.NewRecorder()
	bridge.Callback(freshReplay, freshReplayRequest)
	if freshReplay.Code != http.StatusBadRequest || foloAuth.applyCalls != 1 {
		t.Fatalf("replayed token response=%d applyCalls=%d", freshReplay.Code, foloAuth.applyCalls)
	}
}

func TestAuthCallbackRejectsWrongFlowBeforeUpstream(t *testing.T) {
	logs := &bytes.Buffer{}
	secrets := &fakeSecrets{values: map[string]string{}}
	bridge, foloAuth, sessions := newBridge(t, secrets, logs)

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3000/auth/folo/callback?token=one-time-token-1234567890", nil)
	request.Host = "127.0.0.1:3000"
	request.AddCookie(&http.Cookie{Name: auth.FlowCookieName, Value: "wrong-flow-cookie"})
	response := httptest.NewRecorder()
	bridge.Callback(response, request)

	if response.Code != http.StatusBadRequest || foloAuth.applyCalls != 0 || sessions.Len() != 0 {
		t.Fatalf("wrong flow response=%d applyCalls=%d sessions=%d", response.Code, foloAuth.applyCalls, sessions.Len())
	}
}

func TestAuthStartRateLimitUsesRemoteAddressOnly(t *testing.T) {
	logs := &bytes.Buffer{}
	bridge, _, _ := newBridge(t, &fakeSecrets{values: map[string]string{}}, logs)

	for index := 0; index < 11; index++ {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3000/auth/folo/start", nil)
		request.Host = "127.0.0.1:3000"
		request.RemoteAddr = "127.0.0.1:45678"
		request.Header.Set("X-Forwarded-For", "198.51.100."+string(rune('A'+index)))
		response := httptest.NewRecorder()
		bridge.Start(response, request)
		if index < 10 && response.Code != http.StatusFound {
			t.Fatalf("attempt %d status = %d", index+1, response.Code)
		}
		if index == 10 && response.Code != http.StatusTooManyRequests {
			t.Fatalf("rate limited status = %d", response.Code)
		}
	}
}

func TestLogoutDeletesLocalSessionAndKeychainToken(t *testing.T) {
	logs := &bytes.Buffer{}
	secrets := &fakeSecrets{values: map[string]string{}}
	bridge, foloAuth, sessions := newBridge(t, secrets, logs)
	raw, err := session.NewToken()
	if err != nil {
		t.Fatalf("new session token: %v", err)
	}
	record, err := sessions.Create(context.Background(), raw, session.User{ID: "user_1", Name: "Test"}, time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	secrets.values[record.IDHash] = foloAuth.token
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:3000/auth/logout", nil)
	request = request.WithContext(session.WithRecord(request.Context(), record))
	response := httptest.NewRecorder()

	bridge.Logout(response, request)

	if response.Code != http.StatusNoContent || sessions.Len() != 0 {
		t.Fatalf("logout response=%d sessions=%d", response.Code, sessions.Len())
	}
	if _, exists := secrets.values[record.IDHash]; exists {
		t.Fatal("upstream token remains in secret store")
	}
	if foloAuth.signOutCalls != 1 || foloAuth.signedOutToken != foloAuth.token {
		t.Fatalf("signout calls=%d token=%q", foloAuth.signOutCalls, foloAuth.signedOutToken)
	}
	if strings.Contains(response.Header().Get("Set-Cookie"), foloAuth.token) || strings.Contains(logs.String(), foloAuth.token) {
		t.Fatal("logout exposed upstream token")
	}
}

func TestAuthCallbackDoesNotCreateSessionWhenSecretStoreFails(t *testing.T) {
	logs := &bytes.Buffer{}
	secrets := &fakeSecrets{values: map[string]string{}, setError: errors.New("keychain unavailable")}
	bridge, _, sessions := newBridge(t, secrets, logs)
	flowCookie := startFlow(t, bridge)

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3000/auth/folo/callback?token=one-time-token-1234567890", nil)
	request.Host = "127.0.0.1:3000"
	request.AddCookie(flowCookie)
	response := httptest.NewRecorder()
	bridge.Callback(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("callback status = %d", response.Code)
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == session.LocalCookieName && cookie.Value != "" {
			t.Fatal("local session cookie created after Keychain failure")
		}
	}
	if sessions.Len() != 0 {
		t.Fatal("local session stored after Keychain failure")
	}
}

func TestAuthCallbackRollsBackKeychainWhenSessionStoreFails(t *testing.T) {
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	logs := &bytes.Buffer{}
	secrets := &fakeSecrets{values: map[string]string{}}
	sessions, err := session.NewStoreWithBackend(func() time.Time { return now }, failingSessionBackend{})
	if err != nil {
		t.Fatalf("create session store: %v", err)
	}
	foloAuth := &fakeFoloAuth{
		token: "folo-session-CANARY-auth",
		user:  session.User{ID: "user_1", Name: "Test User"},
	}
	bridge, err := auth.NewBridge(auth.Config{
		FoloWebURL:  "https://app.folo.is",
		CallbackURL: "http://127.0.0.1:3000/auth/folo/callback",
		Now:         func() time.Time { return now },
		Logger:      slog.New(slog.NewJSONHandler(logs, nil)),
		Sessions:    sessions,
		Secrets:     secrets,
		Folo:        foloAuth,
	})
	if err != nil {
		t.Fatalf("create bridge: %v", err)
	}
	flowCookie := startFlow(t, bridge)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3000/auth/folo/callback?token=one-time-token-1234567890", nil)
	request.Host = "127.0.0.1:3000"
	request.AddCookie(flowCookie)
	response := httptest.NewRecorder()
	bridge.Callback(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("callback status=%d", response.Code)
	}
	if len(secrets.values) != 0 {
		t.Fatalf("orphaned Keychain values=%v", secrets.values)
	}
	if foloAuth.signOutCalls != 1 {
		t.Fatalf("upstream session was not rolled back: signOutCalls=%d", foloAuth.signOutCalls)
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == session.LocalCookieName && cookie.Value != "" {
			t.Fatal("local session cookie created after session database failure")
		}
	}
	if strings.Contains(response.Body.String(), foloAuth.token) || strings.Contains(logs.String(), foloAuth.token) {
		t.Fatal("session rollback exposed upstream token")
	}
}

func TestAuthCallbackRejectsUnsafeUpstreamCookieValue(t *testing.T) {
	logs := &bytes.Buffer{}
	secrets := &fakeSecrets{values: map[string]string{}}
	bridge, foloAuth, sessions := newBridge(t, secrets, logs)
	foloAuth.token = "unsafe;upstream-token"
	flowCookie := startFlow(t, bridge)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3000/auth/folo/callback?token=one-time-token-1234567890", nil)
	request.Host = "127.0.0.1:3000"
	request.AddCookie(flowCookie)
	response := httptest.NewRecorder()
	bridge.Callback(response, request)

	if response.Code != http.StatusBadGateway || sessions.Len() != 0 || len(secrets.values) != 0 {
		t.Fatalf("unsafe upstream token response=%d sessions=%d secrets=%d", response.Code, sessions.Len(), len(secrets.values))
	}
	if strings.Contains(response.Body.String(), foloAuth.token) || strings.Contains(logs.String(), foloAuth.token) {
		t.Fatal("unsafe upstream token was exposed")
	}
}
