package http

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/url"
	"strings"
	"time"

	"tantan.local/tantan-api/internal/auth"
	"tantan.local/tantan-api/internal/folo"
	"tantan.local/tantan-api/internal/session"
)

const version = "dev"

var allowedHosts = map[string]struct{}{
	"127.0.0.1:3000": {},
	"localhost:3000": {},
}

var allowedOrigins = map[string]struct{}{
	"http://127.0.0.1:3000": {},
	"http://127.0.0.1:5173": {},
	"http://localhost:3000": {},
	"http://localhost:5173": {},
}

type RouterConfig struct {
	Auth     *auth.Bridge
	Proxy    *folo.Proxy
	Sessions *session.Store
	Local    stdhttp.Handler
	Health   stdhttp.Handler
	Logger   *slog.Logger
}

type router struct {
	auth     *auth.Bridge
	proxy    *folo.Proxy
	sessions *session.Store
	local    stdhttp.Handler
	health   stdhttp.Handler
	logger   *slog.Logger
}

func NewRouter(config RouterConfig) stdhttp.Handler {
	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	health := config.Health
	if health == nil {
		health = defaultHealthHandler()
	}
	return &router{
		auth:     config.Auth,
		proxy:    config.Proxy,
		sessions: config.Sessions,
		local:    config.Local,
		health:   health,
		logger:   logger,
	}
}

func ValidateListenAddr(address string) error {
	if address != "127.0.0.1:3000" {
		return errors.New("listen address must be exactly 127.0.0.1:3000")
	}
	return nil
}

func (router *router) ServeHTTP(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	requestID := newRequestID()
	request.Header.Set("X-Request-Id", requestID)
	writer.Header().Set("X-Request-Id", requestID)
	startedAt := time.Now()
	recorder := &statusRecorder{ResponseWriter: writer, status: stdhttp.StatusOK}

	if _, ok := allowedHosts[request.Host]; !ok {
		writeError(recorder, requestID, stdhttp.StatusForbidden, "ORIGIN_REJECTED", "请求来源不受信任")
		router.logRequest(request, recorder.status, startedAt, "ORIGIN_REJECTED")
		return
	}

	origin := request.Header.Get("Origin")
	if origin != "" {
		if _, ok := allowedOrigins[origin]; !ok {
			writeError(recorder, requestID, stdhttp.StatusForbidden, "ORIGIN_REJECTED", "请求来源不受信任")
			router.logRequest(request, recorder.status, startedAt, "ORIGIN_REJECTED")
			return
		}
		recorder.Header().Set("Access-Control-Allow-Origin", origin)
		recorder.Header().Set("Access-Control-Allow-Credentials", "true")
		recorder.Header().Set("Vary", "Origin")
	}
	if request.Method == stdhttp.MethodOptions {
		if origin == "" {
			writeError(recorder, requestID, stdhttp.StatusForbidden, "ORIGIN_REJECTED", "请求来源不受信任")
			router.logRequest(request, recorder.status, startedAt, "ORIGIN_REJECTED")
			return
		}
		requestedMethod := strings.ToUpper(strings.TrimSpace(request.Header.Get("Access-Control-Request-Method")))
		if !router.allowsPreflight(requestedMethod, request.URL.EscapedPath()) || !allowedPreflightHeaders(request.Header.Get("Access-Control-Request-Headers")) {
			writeError(recorder, requestID, stdhttp.StatusForbidden, "ORIGIN_REJECTED", "跨域预检请求未获允许")
			router.logRequest(request, recorder.status, startedAt, "ORIGIN_REJECTED")
			return
		}
		recorder.Header().Set("Vary", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers")
		recorder.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key, X-Tantan-Timezone")
		recorder.Header().Set("Access-Control-Allow-Methods", requestedMethod)
		recorder.WriteHeader(stdhttp.StatusNoContent)
		router.logRequest(request, recorder.status, startedAt, "")
		return
	}
	if isLocalMutation(request.Method, request.URL.Path) && origin == "" {
		writeError(recorder, requestID, stdhttp.StatusForbidden, "ORIGIN_REJECTED", "请求来源不受信任")
		router.logRequest(request, recorder.status, startedAt, "ORIGIN_REJECTED")
		return
	}

	request, err := router.withOptionalSession(request)
	if err != nil {
		status := stdhttp.StatusInternalServerError
		code := "LOCAL_STORAGE_ERROR"
		message := "本地会话存储不可用"
		if errors.Is(err, errInvalidTimezone) {
			status = stdhttp.StatusBadRequest
			code = "VALIDATION_ERROR"
			message = "时区无效"
		}
		writeError(recorder, requestID, status, code, message)
		router.logRequest(request, recorder.status, startedAt, code)
		return
	}
	if strings.HasPrefix(request.URL.Path, "/tantan/v1/") {
		if _, ok := session.FromContext(request.Context()); !ok {
			writeError(recorder, requestID, stdhttp.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
			router.logRequest(request, recorder.status, startedAt, "AUTH_REQUIRED")
			return
		}
	}
	request = sanitizeBrowserCredentials(request)
	errorCode := router.dispatch(recorder, request, requestID)
	router.logRequest(request, recorder.status, startedAt, errorCode)
}

func (router *router) allowsPreflight(method, escapedPath string) bool {
	if method == "" || strings.ContainsAny(method, " \t\r\n") {
		return false
	}
	path, err := url.PathUnescape(escapedPath)
	if err != nil {
		return false
	}
	switch {
	case method == stdhttp.MethodPost && path == "/auth/logout":
		return true
	case method == stdhttp.MethodGet && (path == "/tantan/v1/session" || path == "/healthz" || path == "/readyz"):
		return true
	case strings.HasPrefix(path, "/tantan/v1/"):
		authorizer, ok := router.local.(PreflightAuthorizer)
		return ok && authorizer.AllowsPreflight(method, path)
	case router.proxy != nil:
		return router.proxy.Decide(method, escapedPath).Kind == folo.DecisionAllow
	default:
		return false
	}
}

func allowedPreflightHeaders(raw string) bool {
	allowed := map[string]struct{}{
		"content-type":      {},
		"idempotency-key":   {},
		"x-tantan-timezone": {},
	}
	for _, value := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(value))
		if name == "" {
			continue
		}
		if _, ok := allowed[name]; !ok {
			return false
		}
	}
	return true
}

func (router *router) dispatch(writer stdhttp.ResponseWriter, request *stdhttp.Request, requestID string) string {
	switch {
	case request.URL.Path == "/healthz" || request.URL.Path == "/readyz":
		router.health.ServeHTTP(writer, request)
		return ""
	case request.Method == stdhttp.MethodGet && request.URL.Path == "/auth/folo/start" && router.auth != nil:
		router.auth.Start(writer, request)
		return ""
	case request.Method == stdhttp.MethodGet && request.URL.Path == "/auth/folo/callback" && router.auth != nil:
		router.auth.Callback(writer, request)
		return ""
	case request.Method == stdhttp.MethodPost && request.URL.Path == "/auth/logout" && router.auth != nil:
		router.auth.Logout(writer, request)
		return ""
	case request.Method == stdhttp.MethodGet && request.URL.Path == "/tantan/v1/session" && router.auth != nil:
		router.auth.CurrentSession(writer, request)
		return ""
	case strings.HasPrefix(request.URL.Path, "/tantan/v1/") && router.local != nil:
		router.local.ServeHTTP(writer, request)
		return ""
	case strings.HasPrefix(request.URL.Path, "/tantan/v1/"):
		writeError(writer, requestID, stdhttp.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未准备就绪")
		return "SERVICE_NOT_READY"
	case router.proxy != nil:
		router.proxy.ServeHTTP(writer, request)
		return ""
	default:
		writeError(writer, requestID, stdhttp.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未准备就绪")
		return "SERVICE_NOT_READY"
	}
}

func (router *router) withOptionalSession(request *stdhttp.Request) (*stdhttp.Request, error) {
	if router.sessions == nil {
		return request, nil
	}
	cookie, err := request.Cookie(session.LocalCookieName)
	if err != nil || cookie.Value == "" {
		return request, nil
	}
	record, ok, err := router.sessions.LookupRaw(request.Context(), cookie.Value)
	if err != nil {
		return request, err
	}
	if !ok {
		return request, nil
	}
	if timezone := strings.TrimSpace(request.Header.Get("X-Tantan-Timezone")); timezone != "" {
		if len(timezone) > 64 {
			return request, errInvalidTimezone
		}
		if _, err := time.LoadLocation(timezone); err != nil {
			return request, errInvalidTimezone
		}
		record, err = router.sessions.UpdateTimezone(request.Context(), record.IDHash, timezone)
		if err != nil {
			return request, err
		}
	}
	return request.WithContext(session.WithRecord(request.Context(), record)), nil
}

var errInvalidTimezone = errors.New("invalid timezone")

func sanitizeBrowserCredentials(request *stdhttp.Request) *stdhttp.Request {
	sanitized := request.Clone(request.Context())
	for _, name := range []string{
		"Authorization",
		"Cookie",
		"Forwarded",
		"Proxy-Authorization",
		"Referer",
		"X-Forwarded-For",
		"X-Forwarded-Host",
		"X-Forwarded-Proto",
		"X-Real-Ip",
	} {
		sanitized.Header.Del(name)
	}
	if request.URL.Path == "/auth/folo/callback" {
		if flowCookie, err := request.Cookie(auth.FlowCookieName); err == nil {
			sanitized.AddCookie(flowCookie)
		}
	}
	return sanitized
}

func routeLabel(path string) string {
	switch path {
	case "/healthz", "/readyz", "/auth/folo/start", "/auth/folo/callback", "/auth/logout", "/tantan/v1/session":
		return path
	}
	if strings.HasPrefix(path, "/tantan/v1/") {
		return "/tantan/v1/*"
	}
	return "folo-proxy"
}

func (router *router) logRequest(request *stdhttp.Request, status int, startedAt time.Time, errorCode string) {
	attributes := []any{
		slog.String("requestId", request.Header.Get("X-Request-Id")),
		slog.String("module", "http"),
		slog.String("method", request.Method),
		slog.String("route", routeLabel(request.URL.Path)),
		slog.Int("status", status),
		slog.Int64("durationMs", time.Since(startedAt).Milliseconds()),
	}
	if errorCode != "" {
		attributes = append(attributes, slog.String("errorCode", errorCode))
	}
	router.logger.InfoContext(request.Context(), "http_request", attributes...)
}

type statusRecorder struct {
	stdhttp.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *statusRecorder) Write(contents []byte) (int, error) {
	return recorder.ResponseWriter.Write(contents)
}

func isLocalMutation(method, path string) bool {
	if method != stdhttp.MethodPost && method != stdhttp.MethodPut && method != stdhttp.MethodPatch && method != stdhttp.MethodDelete {
		return false
	}
	return path == "/auth/logout" || strings.HasPrefix(path, "/tantan/v1/")
}

func newRequestID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err == nil {
		return hex.EncodeToString(buffer)
	}
	return "local-request-id"
}

func writeJSON(writer stdhttp.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer stdhttp.ResponseWriter, requestID string, status int, code, message string) {
	writeJSON(writer, status, map[string]any{
		"requestId": requestID,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
