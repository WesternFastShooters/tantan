package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"tantan.local/tantan-api/internal/api/gen"
	"tantan.local/tantan-api/internal/folo"
	"tantan.local/tantan-api/internal/session"
)

const (
	maximumAuthBodyBytes = 8 * 1024
	twoFactorTTL         = 5 * time.Minute
	tokenReplayTTL       = 24 * time.Hour
)

var (
	emailPattern    = regexp.MustCompile(`^[^\s@]+@[^\s@]+$`)
	returnToPattern = regexp.MustCompile(`^/[A-Za-z0-9/_?&=.%:-]*$`)
	totpPattern     = regexp.MustCompile(`^[0-9]{6,10}$`)
)

type FoloAuth interface {
	ApplyOneTimeToken(ctx context.Context, token string) (string, session.User, error)
	SignInEmail(ctx context.Context, email, password string) (folo.LoginResult, error)
	VerifyTOTP(ctx context.Context, pendingCookie, code string) (folo.LoginResult, error)
	SignOut(ctx context.Context, upstreamToken string) error
}

type TokenReplayStore interface {
	Reserve(ctx context.Context, tokenHash string, expiresAt time.Time) (bool, error)
	Release(ctx context.Context, tokenHash string) error
}

type Config struct {
	PublicOrigin string
	FoloWebURL   string
	Now          func() time.Time
	Logger       *slog.Logger
	Sessions     *session.Store
	Secrets      session.SecretStore
	Replays      TokenReplayStore
	Folo         FoloAuth
}

type pendingTwoFactor struct {
	cookie    string
	expiresAt time.Time
	inUse     bool
	attempts  int
}

type Bridge struct {
	publicOrigin *url.URL
	foloWebURL   *url.URL
	now          func() time.Time
	logger       *slog.Logger
	sessions     *session.Store
	secrets      session.SecretStore
	replays      TokenReplayStore
	folo         FoloAuth

	mu        sync.Mutex
	twoFactor map[string]pendingTwoFactor
	attempts  map[string][]time.Time
}

func NewBridge(config Config) (*Bridge, error) {
	if config.Sessions == nil || config.Secrets == nil || config.Replays == nil || config.Folo == nil {
		return nil, errors.New("auth session, secret, replay and Folo dependencies are required")
	}
	publicOrigin, err := url.Parse(config.PublicOrigin)
	if err != nil || !allowedPublicOrigin(publicOrigin) {
		return nil, errors.New("public origin must be HTTPS or a loopback HTTP test origin")
	}
	publicOrigin.Path = ""
	publicOrigin.RawPath = ""
	foloWebURL, err := url.Parse(config.FoloWebURL)
	if err != nil || !allowedFoloWebURL(foloWebURL) {
		return nil, errors.New("Folo web URL must be https://app.folo.is or a loopback test server")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	return &Bridge{
		publicOrigin: publicOrigin,
		foloWebURL:   foloWebURL,
		now:          now,
		logger:       logger,
		sessions:     config.Sessions,
		secrets:      config.Secrets,
		replays:      config.Replays,
		folo:         config.Folo,
		twoFactor:    make(map[string]pendingTwoFactor),
		attempts:     make(map[string][]time.Time),
	}, nil
}

func (bridge *Bridge) Providers(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, gen.FoloAuthProvidersResponse{Providers: []gen.FoloAuthProvider{
		gen.FoloAuthProviderGoogle,
		gen.FoloAuthProviderGithub,
		gen.FoloAuthProviderApple,
		gen.FoloAuthProviderCredential,
		gen.FoloAuthProviderToken,
	}})
}

func (bridge *Bridge) SocialStart(writer http.ResponseWriter, request *http.Request) {
	if !bridge.allowAuthRequest(writer, request) {
		return
	}
	var body gen.FoloSocialStartRequest
	if err := decodeJSON(request, &body); err != nil {
		writeError(writer, request, http.StatusBadRequest, gen.ErrorCodeAuthProviderInvalid, "登录方式无效", false)
		return
	}
	provider := string(body.Provider)
	if provider != "google" && provider != "github" && provider != "apple" {
		writeError(writer, request, http.StatusBadRequest, gen.ErrorCodeAuthProviderInvalid, "登录方式无效", false)
		return
	}
	loginURL := *bridge.foloWebURL
	loginURL.Path = "/login"
	loginURL.RawQuery = url.Values{"provider": []string{provider}}.Encode()
	writeJSON(writer, http.StatusOK, gen.FoloSocialStartResponse{AuthorizeURL: loginURL.String(), Handoff: "one-time-token"})
}

func (bridge *Bridge) Token(writer http.ResponseWriter, request *http.Request) {
	if !bridge.allowAuthRequest(writer, request) {
		return
	}
	var body gen.FoloTokenLoginRequest
	if err := decodeJSON(request, &body); err != nil || !validReturnTo(body.ReturnTo) {
		writeError(writer, request, http.StatusBadRequest, gen.ErrorCodeValidationError, "授权令牌格式无效", false)
		return
	}
	token, err := normalizeAuthorizationToken(body.Token)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, gen.ErrorCodeValidationError, "授权令牌格式无效", false)
		return
	}
	tokenHash := hashValue(token)
	reserved, err := bridge.replays.Reserve(request.Context(), tokenHash, bridge.now().UTC().Add(tokenReplayTTL))
	if err != nil {
		writeError(writer, request, http.StatusInternalServerError, gen.ErrorCodeLocalStorageError, "登录存储暂时不可用", true)
		return
	}
	if !reserved {
		writeError(writer, request, http.StatusConflict, gen.ErrorCodeAuthTokenUsed, "授权令牌无效或已使用", false)
		return
	}
	upstreamToken, user, err := bridge.folo.ApplyOneTimeToken(request.Context(), token)
	if err != nil {
		_ = bridge.replays.Release(request.Context(), tokenHash)
		bridge.writeUpstreamAuthError(writer, request, err)
		return
	}
	if err := bridge.createSession(writer, request, upstreamToken, user); err != nil {
		_ = bridge.replays.Release(request.Context(), tokenHash)
		bridge.writeSessionError(writer, request, upstreamToken, err)
	}
}

func (bridge *Bridge) Email(writer http.ResponseWriter, request *http.Request) {
	if !bridge.allowAuthRequest(writer, request) {
		return
	}
	var body gen.FoloEmailLoginRequest
	if err := decodeJSON(request, &body); err != nil || !validEmailLogin(body) {
		writeError(writer, request, http.StatusBadRequest, gen.ErrorCodeValidationError, "邮箱登录信息无效", false)
		return
	}
	result, err := bridge.folo.SignInEmail(request.Context(), body.Email, body.Password)
	body.Password = ""
	if err != nil {
		bridge.writeUpstreamAuthError(writer, request, err)
		return
	}
	if result.TwoFactorRequired {
		flowID, tokenErr := session.NewToken()
		if tokenErr != nil {
			writeError(writer, request, http.StatusInternalServerError, gen.ErrorCodeLocalStorageError, "无法建立两步验证流程", true)
			return
		}
		expiresAt := bridge.now().UTC().Add(twoFactorTTL)
		bridge.mu.Lock()
		bridge.pruneLocked()
		bridge.twoFactor[flowID] = pendingTwoFactor{cookie: result.PendingCookie, expiresAt: expiresAt}
		bridge.mu.Unlock()
		writeJSON(writer, http.StatusConflict, gen.FoloTwoFactorChallengeResponse{
			RequestID: requestID(request),
			Error:     gen.ErrorObject{Code: gen.ErrorCodeAuth2faRequired, Message: "请输入 Folo 两步验证码", Retryable: false},
			Challenge: gen.FoloTwoFactorChallenge{FlowID: gen.Identifier(flowID), ExpiresAt: expiresAt.Format(time.RFC3339Nano)},
		})
		return
	}
	if err := bridge.createSession(writer, request, result.SessionToken, result.User); err != nil {
		bridge.writeSessionError(writer, request, result.SessionToken, err)
	}
}

func (bridge *Bridge) TwoFactor(writer http.ResponseWriter, request *http.Request) {
	if !bridge.allowAuthRequest(writer, request) {
		return
	}
	var body gen.FoloTwoFactorVerifyRequest
	if err := decodeJSON(request, &body); err != nil || !totpPattern.MatchString(body.Code) {
		writeError(writer, request, http.StatusBadRequest, gen.ErrorCodeValidationError, "两步验证码格式无效", false)
		return
	}
	flowID := string(body.FlowID)
	pending, ok := bridge.acquireTwoFactor(flowID)
	if !ok {
		writeError(writer, request, http.StatusGone, gen.ErrorCodeAuthFlowInvalid, "两步验证已过期，请重新登录", false)
		return
	}
	result, err := bridge.folo.VerifyTOTP(request.Context(), pending.cookie, body.Code)
	body.Code = ""
	if err != nil {
		bridge.finishTwoFactor(flowID, false)
		bridge.writeUpstreamAuthError(writer, request, err)
		return
	}
	bridge.finishTwoFactor(flowID, true)
	if err := bridge.createSession(writer, request, result.SessionToken, result.User); err != nil {
		bridge.writeSessionError(writer, request, result.SessionToken, err)
	}
}

func (bridge *Bridge) Logout(writer http.ResponseWriter, request *http.Request) {
	record, ok := session.FromContext(request.Context())
	if ok {
		upstreamToken, _ := bridge.secrets.Get(request.Context(), record.SecretRef)
		if err := bridge.secrets.Delete(request.Context(), record.SecretRef); err != nil {
			bridge.logger.ErrorContext(request.Context(), "auth_secret_delete_failed", slog.String("errorCode", "LOCAL_STORAGE_ERROR"))
		}
		if err := bridge.sessions.DeleteHash(request.Context(), record.IDHash); err != nil {
			bridge.logger.ErrorContext(request.Context(), "auth_session_delete_failed", slog.String("errorCode", "LOCAL_STORAGE_ERROR"))
		}
		if safeCookieValue(upstreamToken) {
			signOutContext, cancel := context.WithTimeout(request.Context(), 5*time.Second)
			_ = bridge.folo.SignOut(signOutContext, upstreamToken)
			cancel()
		}
	}
	bridge.clearSessionCookie(writer)
	writer.WriteHeader(http.StatusNoContent)
}

func (bridge *Bridge) CurrentSession(writer http.ResponseWriter, request *http.Request) {
	record, ok := session.FromContext(request.Context())
	if !ok {
		writeError(writer, request, http.StatusUnauthorized, gen.ErrorCodeAuthRequired, "请先登录", false)
		return
	}
	csrf, err := session.NewCSRFToken()
	if err != nil {
		writeError(writer, request, http.StatusInternalServerError, gen.ErrorCodeLocalStorageError, "无法刷新安全令牌", true)
		return
	}
	record, err = bridge.sessions.RotateCSRF(request.Context(), record.IDHash, csrf)
	if err != nil {
		writeError(writer, request, http.StatusInternalServerError, gen.ErrorCodeLocalStorageError, "无法刷新安全令牌", true)
		return
	}
	writeSession(writer, record, csrf)
}

func (bridge *Bridge) createSession(writer http.ResponseWriter, request *http.Request, upstreamToken string, user session.User) error {
	if !safeCookieValue(upstreamToken) || strings.TrimSpace(user.ID) == "" || strings.TrimSpace(user.Name) == "" {
		return folo.ErrAuthUnavailable
	}
	localToken, err := session.NewToken()
	if err != nil {
		return err
	}
	csrf, err := session.NewCSRFToken()
	if err != nil {
		return err
	}
	idHash := session.HashToken(localToken)
	if err := bridge.secrets.Set(request.Context(), idHash, upstreamToken); err != nil {
		return err
	}
	record, err := bridge.sessions.CreateWithCSRF(request.Context(), localToken, csrf, user, bridge.now().UTC().Add(30*24*time.Hour))
	if err != nil {
		_ = bridge.secrets.Delete(request.Context(), idHash)
		return err
	}
	http.SetCookie(writer, &http.Cookie{
		Name:     session.LocalCookieName,
		Value:    localToken,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
	})
	writeSession(writer, record, csrf)
	return nil
}

func (bridge *Bridge) writeSessionError(writer http.ResponseWriter, request *http.Request, upstreamToken string, err error) {
	if safeCookieValue(upstreamToken) {
		contextWithTimeout, cancel := context.WithTimeout(request.Context(), 5*time.Second)
		_ = bridge.folo.SignOut(contextWithTimeout, upstreamToken)
		cancel()
	}
	bridge.logger.ErrorContext(request.Context(), "auth_session_create_failed", slog.String("errorCode", "LOCAL_STORAGE_ERROR"))
	writeError(writer, request, http.StatusInternalServerError, gen.ErrorCodeLocalStorageError, "无法创建安全会话", true)
}

func (bridge *Bridge) writeUpstreamAuthError(writer http.ResponseWriter, request *http.Request, err error) {
	if errors.Is(err, folo.ErrAuthInvalid) {
		writeError(writer, request, http.StatusUnauthorized, gen.ErrorCodeAuthInvalid, "登录信息无效", false)
		return
	}
	bridge.logger.WarnContext(request.Context(), "folo_auth_failed", slog.String("errorCode", "FOLO_UNAVAILABLE"))
	writeError(writer, request, http.StatusServiceUnavailable, gen.ErrorCodeFoloUnavailable, "Folo 登录暂时不可用", true)
}

func (bridge *Bridge) allowAuthRequest(writer http.ResponseWriter, request *http.Request) bool {
	if request.Header.Get("Origin") != bridge.publicOrigin.String() || request.Host != bridge.publicOrigin.Host {
		writeError(writer, request, http.StatusForbidden, gen.ErrorCodeOriginRejected, "请求来源不受信任", false)
		return false
	}
	if !bridge.allowAttempt(request.RemoteAddr) {
		writer.Header().Set("Retry-After", "60")
		writeError(writer, request, http.StatusTooManyRequests, gen.ErrorCodeRateLimited, "登录请求过于频繁，请稍后重试", true)
		return false
	}
	return true
}

func (bridge *Bridge) allowAttempt(remoteAddress string) bool {
	host := remoteAddress
	if parsedHost, _, err := net.SplitHostPort(remoteAddress); err == nil {
		host = parsedHost
	}
	now := bridge.now().UTC()
	cutoff := now.Add(-time.Minute)
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	attempts := bridge.attempts[host]
	kept := attempts[:0]
	for _, attemptedAt := range attempts {
		if attemptedAt.After(cutoff) {
			kept = append(kept, attemptedAt)
		}
	}
	if len(kept) >= 10 {
		bridge.attempts[host] = kept
		return false
	}
	bridge.attempts[host] = append(kept, now)
	return true
}

func (bridge *Bridge) acquireTwoFactor(flowID string) (pendingTwoFactor, bool) {
	now := bridge.now().UTC()
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	bridge.pruneLocked()
	pending, ok := bridge.twoFactor[flowID]
	if !ok || pending.inUse || !pending.expiresAt.After(now) || pending.attempts >= 5 {
		return pendingTwoFactor{}, false
	}
	pending.inUse = true
	pending.attempts++
	bridge.twoFactor[flowID] = pending
	return pending, true
}

func (bridge *Bridge) finishTwoFactor(flowID string, succeeded bool) {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	pending, ok := bridge.twoFactor[flowID]
	if !ok {
		return
	}
	if succeeded || pending.attempts >= 5 {
		delete(bridge.twoFactor, flowID)
		return
	}
	pending.inUse = false
	bridge.twoFactor[flowID] = pending
}

func (bridge *Bridge) pruneLocked() {
	now := bridge.now().UTC()
	for flowID, pending := range bridge.twoFactor {
		if !pending.expiresAt.After(now) {
			delete(bridge.twoFactor, flowID)
		}
	}
}

func (bridge *Bridge) clearSessionCookie(writer http.ResponseWriter) {
	http.SetCookie(writer, &http.Cookie{
		Name:     session.LocalCookieName,
		Value:    "",
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func normalizeAuthorizationToken(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if strings.ContainsAny(value, "\r\n\x00") {
		return "", errors.New("invalid authorization token")
	}
	if parsed, err := url.Parse(value); err == nil && parsed.RawQuery != "" {
		if parsed.Scheme == "folo" && parsed.Host == "auth" || parsed.Scheme == "" && strings.Trim(parsed.Path, "/") == "auth" {
			value = parsed.Query().Get("token")
		}
	}
	if len(value) < 20 || len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
		return "", errors.New("invalid authorization token")
	}
	return value, nil
}

func validEmailLogin(body gen.FoloEmailLoginRequest) bool {
	return len(body.Email) <= 254 && emailPattern.MatchString(body.Email) && len(body.Password) >= 8 && len(body.Password) <= 128 && !strings.ContainsAny(body.Password, "\r\n\x00") && validReturnTo(body.ReturnTo)
}

func validReturnTo(value *string) bool {
	return value == nil || len(*value) <= 1024 && returnToPattern.MatchString(*value)
}

func decodeJSON(request *http.Request, destination any) error {
	if request.Body == nil {
		return errors.New("request body is required")
	}
	decoder := json.NewDecoder(io.LimitReader(request.Body, maximumAuthBodyBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func safeCookieValue(value string) bool {
	if len(value) < 1 || len(value) > 4096 {
		return false
	}
	for index := range len(value) {
		character := value[index]
		if character < 0x21 || character > 0x7e || character == '"' || character == ',' || character == ';' || character == '\\' {
			return false
		}
	}
	return true
}

func hashValue(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

func allowedPublicOrigin(value *url.URL) bool {
	if value == nil || value.User != nil || value.RawQuery != "" || value.Fragment != "" || (value.Path != "" && value.Path != "/") || value.Host == "" {
		return false
	}
	if value.Scheme == "https" {
		return true
	}
	return value.Scheme == "http" && (value.Hostname() == "127.0.0.1" || value.Hostname() == "localhost")
}

func allowedFoloWebURL(value *url.URL) bool {
	if value == nil || value.User != nil || value.RawQuery != "" || value.Fragment != "" || (value.Path != "" && value.Path != "/") {
		return false
	}
	if value.Scheme == "https" && value.Host == "app.folo.is" {
		return true
	}
	return value.Scheme == "http" && (value.Hostname() == "127.0.0.1" || value.Hostname() == "localhost")
}

func writeSession(writer http.ResponseWriter, record session.Record, csrf string) {
	writeJSON(writer, http.StatusOK, gen.SessionResponse{User: gen.SessionUser{
		ID: gen.Identifier(record.User.ID), Name: record.User.Name, Email: record.User.Email, Image: record.User.Image,
	}, Timezone: record.Timezone, CSRFToken: csrf})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, request *http.Request, status int, code gen.ErrorCode, message string, retryable bool) {
	requestID := requestID(request)
	writer.Header().Set("X-Request-Id", requestID)
	writeJSON(writer, status, gen.ErrorResponse{RequestID: requestID, Error: gen.ErrorObject{Code: code, Message: message, Retryable: retryable}})
}

func requestID(request *http.Request) string {
	requestID := strings.TrimSpace(request.Header.Get("X-Request-Id"))
	if len(requestID) < 8 {
		return "local-request"
	}
	return requestID
}
