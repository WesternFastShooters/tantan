package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/api/gen"
	"tantan.local/tantan-api/internal/auth"
	"tantan.local/tantan-api/internal/folo"
	"tantan.local/tantan-api/internal/session"
)

type fakeFoloAuth struct {
	applyCalls     int
	emailCalls     int
	verifyCalls    int
	signOutCalls   int
	lastToken      string
	lastPassword   string
	lastPending    string
	lastCode       string
	applyToken     string
	applyUser      session.User
	emailResult    folo.LoginResult
	verifyResult   folo.LoginResult
	applyError     error
	emailError     error
	verifyError    error
	signedOutToken string
}

func (fake *fakeFoloAuth) ApplyOneTimeToken(_ context.Context, token string) (string, session.User, error) {
	fake.applyCalls++
	fake.lastToken = token
	return fake.applyToken, fake.applyUser, fake.applyError
}

func (fake *fakeFoloAuth) SignInEmail(_ context.Context, _ string, password string) (folo.LoginResult, error) {
	fake.emailCalls++
	fake.lastPassword = password
	return fake.emailResult, fake.emailError
}

func (fake *fakeFoloAuth) VerifyTOTP(_ context.Context, pendingCookie, code string) (folo.LoginResult, error) {
	fake.verifyCalls++
	fake.lastPending = pendingCookie
	fake.lastCode = code
	return fake.verifyResult, fake.verifyError
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

func (store *fakeSecrets) Get(_ context.Context, account string) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	value, ok := store.values[account]
	if !ok {
		return "", session.ErrSecretNotFound
	}
	return value, nil
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

type fakeReplays struct {
	mu     sync.Mutex
	values map[string]time.Time
}

func (store *fakeReplays) Reserve(_ context.Context, tokenHash string, expiresAt time.Time) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.values[tokenHash]; ok {
		return false, nil
	}
	store.values[tokenHash] = expiresAt
	return true, nil
}

func (store *fakeReplays) Release(_ context.Context, tokenHash string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.values, tokenHash)
	return nil
}

func newBridge(t *testing.T, foloAuth *fakeFoloAuth, secrets *fakeSecrets, logs *bytes.Buffer) (*auth.Bridge, *session.Store) {
	t.Helper()
	now := time.Date(2026, 8, 10, 2, 0, 0, 0, time.UTC)
	sessions := session.NewStore(func() time.Time { return now })
	bridge, err := auth.NewBridge(auth.Config{
		PublicOrigin: "http://127.0.0.1:3000",
		FoloWebURL:   "https://app.folo.is",
		Now:          func() time.Time { return now },
		Logger:       slog.New(slog.NewJSONHandler(logs, nil)),
		Sessions:     sessions,
		Secrets:      secrets,
		Replays:      &fakeReplays{values: map[string]time.Time{}},
		Folo:         foloAuth,
	})
	if err != nil {
		t.Fatal(err)
	}
	return bridge, sessions
}

func postRequest(path, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:3000"+path, strings.NewReader(body))
	request.Host = "127.0.0.1:3000"
	request.Header.Set("Origin", "http://127.0.0.1:3000")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-Id", "1234567890abcdef")
	return request
}

func TestProvidersAndSocialStartMatchFoloLoginMethods(t *testing.T) {
	bridge, _ := newBridge(t, &fakeFoloAuth{}, &fakeSecrets{values: map[string]string{}}, &bytes.Buffer{})
	providers := httptest.NewRecorder()
	bridge.Providers(providers, httptest.NewRequest(http.MethodGet, "/api/auth/folo/providers", nil))
	var providerBody gen.FoloAuthProvidersResponse
	if providers.Code != http.StatusOK || json.Unmarshal(providers.Body.Bytes(), &providerBody) != nil {
		t.Fatalf("providers status=%d body=%s", providers.Code, providers.Body.String())
	}
	want := []gen.FoloAuthProvider{"google", "github", "apple", "credential", "token"}
	if strings.Join(providerStrings(providerBody.Providers), ",") != strings.Join(providerStrings(want), ",") {
		t.Fatalf("providers=%v", providerBody.Providers)
	}

	for _, provider := range []string{"google", "github", "apple"} {
		response := httptest.NewRecorder()
		bridge.SocialStart(response, postRequest("/api/auth/folo/social-start", `{"provider":"`+provider+`"}`))
		var body gen.FoloSocialStartResponse
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.AuthorizeURL != "https://app.folo.is/login?provider="+provider || body.Handoff != "one-time-token" {
			t.Fatalf("provider=%s status=%d body=%s", provider, response.Code, response.Body.String())
		}
	}
	invalid := httptest.NewRecorder()
	bridge.SocialStart(invalid, postRequest("/api/auth/folo/social-start", `{"provider":"facebook"}`))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid provider status=%d", invalid.Code)
	}
}

func TestTokenLoginNormalizesFoloURLCreatesOpaqueSessionAndRejectsReplay(t *testing.T) {
	logs := &bytes.Buffer{}
	secrets := &fakeSecrets{values: map[string]string{}}
	foloAuth := &fakeFoloAuth{applyToken: "upstream-session-CANARY-123456", applyUser: session.User{ID: "user_1", Name: "Token User"}}
	bridge, sessions := newBridge(t, foloAuth, secrets, logs)
	requestBody := `{"token":"folo://auth?token=one-time-token-CANARY-123456","returnTo":"/search?q=ai"}`
	response := httptest.NewRecorder()
	bridge.Token(response, postRequest("/api/auth/folo/token", requestBody))
	if response.Code != http.StatusOK || foloAuth.lastToken != "one-time-token-CANARY-123456" || sessions.Len() != 1 {
		t.Fatalf("status=%d token=%q sessions=%d body=%s", response.Code, foloAuth.lastToken, sessions.Len(), response.Body.String())
	}
	var localCookie *http.Cookie
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == session.LocalCookieName {
			localCookie = cookie
		}
	}
	if localCookie == nil || !localCookie.HttpOnly || !localCookie.Secure || localCookie.Path != "/" {
		t.Fatalf("unsafe session cookie: %#v", localCookie)
	}
	var body gen.SessionResponse
	if json.Unmarshal(response.Body.Bytes(), &body) != nil || body.User.ID != "user_1" || len(body.CSRFToken) < 40 {
		t.Fatalf("session body=%s", response.Body.String())
	}
	record, ok, err := sessions.LookupRaw(context.Background(), localCookie.Value)
	if err != nil || !ok || !session.ValidCSRF(record, body.CSRFToken) || secrets.values[record.SecretRef] != foloAuth.applyToken {
		t.Fatalf("record=%#v ok=%v err=%v", record, ok, err)
	}
	for _, secret := range []string{"one-time-token-CANARY-123456", foloAuth.applyToken} {
		if strings.Contains(response.Body.String(), secret) || strings.Contains(response.Header().Get("Set-Cookie"), secret) || strings.Contains(logs.String(), secret) {
			t.Fatalf("secret reached output: %s", secret)
		}
	}
	replay := httptest.NewRecorder()
	bridge.Token(replay, postRequest("/api/auth/folo/token", requestBody))
	if replay.Code != http.StatusConflict || foloAuth.applyCalls != 1 {
		t.Fatalf("replay status=%d calls=%d", replay.Code, foloAuth.applyCalls)
	}
}

func TestEmailTwoFactorFlowKeepsPendingCookieServerSide(t *testing.T) {
	logs := &bytes.Buffer{}
	secrets := &fakeSecrets{values: map[string]string{}}
	foloAuth := &fakeFoloAuth{
		emailResult:  folo.LoginResult{TwoFactorRequired: true, PendingCookie: "better-auth.two_factor=pending-CANARY-123456"},
		verifyResult: folo.LoginResult{SessionToken: "upstream-email-session-123456", User: session.User{ID: "user_email", Name: "Email User"}},
	}
	bridge, sessions := newBridge(t, foloAuth, secrets, logs)
	email := httptest.NewRecorder()
	bridge.Email(email, postRequest("/api/auth/folo/email", `{"email":"user@example.invalid","password":"correct-password"}`))
	var challenge gen.FoloTwoFactorChallengeResponse
	if email.Code != http.StatusConflict || json.Unmarshal(email.Body.Bytes(), &challenge) != nil || challenge.Error.Code != gen.ErrorCodeAuth2faRequired || challenge.Challenge.FlowID == "" {
		t.Fatalf("email status=%d body=%s", email.Code, email.Body.String())
	}
	if strings.Contains(email.Body.String(), "pending-CANARY") || strings.Contains(logs.String(), "correct-password") {
		t.Fatal("email flow exposed a credential")
	}
	verify := httptest.NewRecorder()
	bridge.TwoFactor(verify, postRequest("/api/auth/folo/two-factor", `{"flowId":"`+string(challenge.Challenge.FlowID)+`","code":"123456"}`))
	if verify.Code != http.StatusOK || sessions.Len() != 1 || foloAuth.lastPending != foloAuth.emailResult.PendingCookie || foloAuth.lastCode != "123456" {
		t.Fatalf("verify status=%d sessions=%d pending=%q code=%q", verify.Code, sessions.Len(), foloAuth.lastPending, foloAuth.lastCode)
	}
	reuse := httptest.NewRecorder()
	bridge.TwoFactor(reuse, postRequest("/api/auth/folo/two-factor", `{"flowId":"`+string(challenge.Challenge.FlowID)+`","code":"123456"}`))
	if reuse.Code != http.StatusGone || foloAuth.verifyCalls != 1 {
		t.Fatalf("reuse status=%d calls=%d", reuse.Code, foloAuth.verifyCalls)
	}
}

func TestAuthRejectsWrongOriginBeforeCallingFolo(t *testing.T) {
	foloAuth := &fakeFoloAuth{}
	bridge, _ := newBridge(t, foloAuth, &fakeSecrets{values: map[string]string{}}, &bytes.Buffer{})
	request := postRequest("/api/auth/folo/email", `{"email":"user@example.invalid","password":"correct-password"}`)
	request.Header.Set("Origin", "https://attacker.invalid")
	response := httptest.NewRecorder()
	bridge.Email(response, request)
	if response.Code != http.StatusForbidden || foloAuth.emailCalls != 0 {
		t.Fatalf("status=%d emailCalls=%d", response.Code, foloAuth.emailCalls)
	}
}

func TestCurrentSessionRotatesCSRFAndLogoutDeletesSealedToken(t *testing.T) {
	secrets := &fakeSecrets{values: map[string]string{}}
	foloAuth := &fakeFoloAuth{}
	bridge, sessions := newBridge(t, foloAuth, secrets, &bytes.Buffer{})
	raw, _ := session.NewToken()
	oldCSRF, _ := session.NewCSRFToken()
	record, err := sessions.CreateWithCSRF(context.Background(), raw, oldCSRF, session.User{ID: "user_1", Name: "User"}, time.Date(2026, 8, 11, 2, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	secrets.values[record.SecretRef] = "upstream-session-token-123456"

	sessionRequest := httptest.NewRequest(http.MethodGet, "/api/tantan/v1/session", nil).WithContext(session.WithRecord(context.Background(), record))
	sessionResponse := httptest.NewRecorder()
	bridge.CurrentSession(sessionResponse, sessionRequest)
	var body gen.SessionResponse
	if sessionResponse.Code != http.StatusOK || json.Unmarshal(sessionResponse.Body.Bytes(), &body) != nil || body.CSRFToken == oldCSRF {
		t.Fatalf("session response=%d body=%s", sessionResponse.Code, sessionResponse.Body.String())
	}
	rotated, ok, _ := sessions.LookupRaw(context.Background(), raw)
	if !ok || !session.ValidCSRF(rotated, body.CSRFToken) || session.ValidCSRF(rotated, oldCSRF) {
		t.Fatal("CSRF was not rotated")
	}

	logoutRequest := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil).WithContext(session.WithRecord(context.Background(), rotated))
	logoutResponse := httptest.NewRecorder()
	bridge.Logout(logoutResponse, logoutRequest)
	if logoutResponse.Code != http.StatusNoContent || sessions.Len() != 0 || len(secrets.values) != 0 || foloAuth.signedOutToken != "upstream-session-token-123456" {
		t.Fatalf("logout status=%d sessions=%d secrets=%d signout=%q", logoutResponse.Code, sessions.Len(), len(secrets.values), foloAuth.signedOutToken)
	}
}

func TestSessionCreationFailureDoesNotExposeOrRetainUpstreamToken(t *testing.T) {
	logs := &bytes.Buffer{}
	secrets := &fakeSecrets{values: map[string]string{}, setError: errors.New("vault unavailable")}
	foloAuth := &fakeFoloAuth{applyToken: "upstream-session-CANARY-123456", applyUser: session.User{ID: "user_1", Name: "Token User"}}
	bridge, sessions := newBridge(t, foloAuth, secrets, logs)
	response := httptest.NewRecorder()
	bridge.Token(response, postRequest("/api/auth/folo/token", `{"token":"one-time-token-CANARY-123456"}`))
	if response.Code != http.StatusInternalServerError || sessions.Len() != 0 || strings.Contains(response.Body.String(), foloAuth.applyToken) || strings.Contains(logs.String(), foloAuth.applyToken) {
		t.Fatalf("status=%d sessions=%d body=%s logs=%s", response.Code, sessions.Len(), response.Body.String(), logs.String())
	}
}

func providerStrings(values []gen.FoloAuthProvider) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return result
}
