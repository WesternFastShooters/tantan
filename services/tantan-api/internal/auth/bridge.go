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
	"strings"
	"sync"
	"time"

	"tantan.local/tantan-api/internal/session"
)

const FlowCookieName = "tantan_auth_flow"

type FoloAuth interface {
	ApplyOneTimeToken(ctx context.Context, oneTimeToken string) (string, session.User, error)
	SignOut(ctx context.Context, upstreamToken string) error
}

type Config struct {
	FoloWebURL  string
	CallbackURL string
	Now         func() time.Time
	Logger      *slog.Logger
	Sessions    *session.Store
	Secrets     session.SecretStore
	Folo        FoloAuth
}

type Bridge struct {
	foloWebURL  *url.URL
	callbackURL *url.URL
	now         func() time.Time
	logger      *slog.Logger
	sessions    *session.Store
	secrets     session.SecretStore
	folo        FoloAuth

	mu         sync.Mutex
	flows      map[string]time.Time
	usedTokens map[string]time.Time
	starts     map[string][]time.Time
}

func NewBridge(config Config) (*Bridge, error) {
	if config.Sessions == nil || config.Secrets == nil || config.Folo == nil {
		return nil, errors.New("auth session, secret store and Folo client are required")
	}
	foloWebURL, err := url.Parse(config.FoloWebURL)
	if err != nil || !allowedFoloWebURL(foloWebURL) {
		return nil, errors.New("Folo web URL must be https://app.folo.is or a loopback test server")
	}
	callbackURL, err := url.Parse(config.CallbackURL)
	if err != nil || callbackURL.Scheme != "http" || callbackURL.Host != "127.0.0.1:3000" || callbackURL.Path != "/auth/folo/callback" {
		return nil, errors.New("auth callback must be the fixed loopback URL")
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
		foloWebURL:  foloWebURL,
		callbackURL: callbackURL,
		now:         now,
		logger:      logger,
		sessions:    config.Sessions,
		secrets:     config.Secrets,
		folo:        config.Folo,
		flows:       make(map[string]time.Time),
		usedTokens:  make(map[string]time.Time),
		starts:      make(map[string][]time.Time),
	}, nil
}

func (bridge *Bridge) Start(writer http.ResponseWriter, request *http.Request) {
	if !allowedHost(request.Host) {
		writeError(writer, request, http.StatusForbidden, "ORIGIN_REJECTED", "请求来源不受信任")
		return
	}
	if !bridge.allowStart(request.RemoteAddr) {
		writer.Header().Set("Retry-After", "60")
		writeError(writer, request, http.StatusTooManyRequests, "RATE_LIMITED", "登录请求过于频繁，请稍后重试")
		return
	}
	flowToken, err := session.NewToken()
	if err != nil {
		writeError(writer, request, http.StatusInternalServerError, "LOCAL_STORAGE_ERROR", "无法开始登录")
		return
	}
	expiresAt := bridge.now().UTC().Add(10 * time.Minute)
	bridge.mu.Lock()
	bridge.pruneLocked()
	bridge.flows[hashValue(flowToken)] = expiresAt
	bridge.mu.Unlock()

	http.SetCookie(writer, &http.Cookie{
		Name:     FlowCookieName,
		Value:    flowToken,
		Path:     "/auth/folo/callback",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	loginURL := *bridge.foloWebURL
	loginURL.Path = "/login"
	query := loginURL.Query()
	query.Set("cli_callback", bridge.callbackURL.String())
	loginURL.RawQuery = query.Encode()
	http.Redirect(writer, request, loginURL.String(), http.StatusFound)
}

func (bridge *Bridge) Callback(writer http.ResponseWriter, request *http.Request) {
	if !allowedHost(request.Host) {
		writeError(writer, request, http.StatusForbidden, "ORIGIN_REJECTED", "请求来源不受信任")
		return
	}
	oneTimeToken := request.URL.Query().Get("token")
	if len(oneTimeToken) < 20 || len(oneTimeToken) > 4096 {
		writeError(writer, request, http.StatusBadRequest, "AUTH_FLOW_INVALID", "登录流程无效，请重试")
		return
	}
	flowCookie, err := request.Cookie(FlowCookieName)
	if err != nil || !bridge.consumeFlow(flowCookie.Value) || bridge.tokenWasUsed(oneTimeToken) {
		bridge.clearFlowCookie(writer)
		writeError(writer, request, http.StatusBadRequest, "AUTH_FLOW_INVALID", "登录流程无效，请重试")
		return
	}
	bridge.clearFlowCookie(writer)

	upstreamToken, user, err := bridge.folo.ApplyOneTimeToken(request.Context(), oneTimeToken)
	if err != nil || !safeCookieValue(upstreamToken) {
		bridge.logger.WarnContext(request.Context(), "auth_callback_failed", slog.String("errorCode", "FOLO_UNAVAILABLE"))
		writeError(writer, request, http.StatusBadGateway, "FOLO_UNAVAILABLE", "Folo 登录暂时不可用")
		return
	}
	bridge.markTokenUsed(oneTimeToken)
	localToken, err := session.NewToken()
	if err != nil {
		writeError(writer, request, http.StatusInternalServerError, "LOCAL_STORAGE_ERROR", "无法创建本地登录")
		return
	}
	idHash := session.HashToken(localToken)
	if err := bridge.secrets.Set(request.Context(), idHash, upstreamToken); err != nil {
		bridge.logger.ErrorContext(request.Context(), "auth_secret_store_failed", slog.String("errorCode", "LOCAL_STORAGE_ERROR"))
		writeError(writer, request, http.StatusInternalServerError, "LOCAL_STORAGE_ERROR", "系统钥匙串不可用")
		return
	}
	expiresAt := bridge.now().UTC().Add(30 * 24 * time.Hour)
	if _, err := bridge.sessions.Create(request.Context(), localToken, user, expiresAt); err != nil {
		_ = bridge.secrets.Delete(request.Context(), idHash)
		_ = bridge.folo.SignOut(request.Context(), upstreamToken)
		bridge.logger.ErrorContext(request.Context(), "auth_session_store_failed", slog.String("errorCode", "LOCAL_STORAGE_ERROR"))
		writeError(writer, request, http.StatusInternalServerError, "LOCAL_STORAGE_ERROR", "本地会话存储不可用")
		return
	}
	http.SetCookie(writer, &http.Cookie{
		Name:     session.LocalCookieName,
		Value:    localToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
	})
	http.Redirect(writer, request, "/", http.StatusFound)
}

func (bridge *Bridge) allowStart(remoteAddress string) bool {
	host := remoteAddress
	if parsedHost, _, err := net.SplitHostPort(remoteAddress); err == nil {
		host = parsedHost
	}
	now := bridge.now().UTC()
	cutoff := now.Add(-time.Minute)
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	attempts := bridge.starts[host]
	kept := attempts[:0]
	for _, attemptedAt := range attempts {
		if attemptedAt.After(cutoff) {
			kept = append(kept, attemptedAt)
		}
	}
	if len(kept) >= 10 {
		bridge.starts[host] = kept
		return false
	}
	bridge.starts[host] = append(kept, now)
	return true
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

func (bridge *Bridge) Logout(writer http.ResponseWriter, request *http.Request) {
	record, ok := session.FromContext(request.Context())
	if ok {
		upstreamToken, _ := bridge.secrets.Get(request.Context(), record.IDHash)
		if err := bridge.secrets.Delete(request.Context(), record.IDHash); err != nil {
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
	http.SetCookie(writer, &http.Cookie{
		Name:     session.LocalCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	writer.WriteHeader(http.StatusNoContent)
}

func (bridge *Bridge) CurrentSession(writer http.ResponseWriter, request *http.Request) {
	record, ok := session.FromContext(request.Context())
	if !ok {
		writeError(writer, request, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"user":     record.User,
		"timezone": record.Timezone,
	})
}

func (bridge *Bridge) consumeFlow(raw string) bool {
	hash := hashValue(raw)
	now := bridge.now().UTC()
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	bridge.pruneLocked()
	expiresAt, ok := bridge.flows[hash]
	delete(bridge.flows, hash)
	return ok && expiresAt.After(now)
}

func (bridge *Bridge) tokenWasUsed(raw string) bool {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	bridge.pruneLocked()
	_, ok := bridge.usedTokens[hashValue(raw)]
	return ok
}

func (bridge *Bridge) markTokenUsed(raw string) {
	bridge.mu.Lock()
	bridge.usedTokens[hashValue(raw)] = bridge.now().UTC().Add(24 * time.Hour)
	bridge.mu.Unlock()
}

func (bridge *Bridge) pruneLocked() {
	now := bridge.now().UTC()
	for hash, expiresAt := range bridge.flows {
		if !expiresAt.After(now) {
			delete(bridge.flows, hash)
		}
	}
	for hash, expiresAt := range bridge.usedTokens {
		if !expiresAt.After(now) {
			delete(bridge.usedTokens, hash)
		}
	}
}

func (bridge *Bridge) clearFlowCookie(writer http.ResponseWriter) {
	http.SetCookie(writer, &http.Cookie{
		Name:     FlowCookieName,
		Value:    "",
		Path:     "/auth/folo/callback",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func hashValue(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

func allowedHost(host string) bool {
	return host == "127.0.0.1:3000" || host == "localhost:3000"
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

func writeError(writer http.ResponseWriter, request *http.Request, status int, code, message string) {
	requestID := strings.TrimSpace(request.Header.Get("X-Request-Id"))
	if len(requestID) < 8 {
		requestID = "local-request"
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("X-Request-Id", requestID)
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"requestId": requestID,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
