package http

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	stdhttp "net/http"
	"net/url"
	"strings"
	"time"

	"tantan.local/tantan-api/internal/auth"
	"tantan.local/tantan-api/internal/folo"
	"tantan.local/tantan-api/internal/session"
)

const (
	version           = "dev"
	defaultOrigin     = "http://127.0.0.1:3000"
	localAPIPrefix    = "/api/tantan/v1/"
	foloAPIPrefix     = "/api/folo"
	maximumHeaderSize = 8 * 1024
)

type RouterConfig struct {
	PublicOrigin      string
	TrustedProxyCIDRs []string
	Auth              *auth.Bridge
	Proxy             *folo.Proxy
	Sessions          *session.Store
	Local             stdhttp.Handler
	Health            stdhttp.Handler
	Static            stdhttp.Handler
	Logger            *slog.Logger
}

type router struct {
	publicOrigin   *url.URL
	trustedProxies []*net.IPNet
	auth           *auth.Bridge
	proxy          *folo.Proxy
	sessions       *session.Store
	local          stdhttp.Handler
	health         stdhttp.Handler
	static         stdhttp.Handler
	logger         *slog.Logger
}

func NewRouter(config RouterConfig) (stdhttp.Handler, error) {
	publicOrigin, err := parsePublicOrigin(config.PublicOrigin)
	if err != nil {
		return nil, err
	}
	trustedProxies, err := parseTrustedProxyCIDRs(config.TrustedProxyCIDRs)
	if err != nil {
		return nil, err
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	health := config.Health
	if health == nil {
		health = defaultHealthHandler()
	}
	return &router{
		publicOrigin:   publicOrigin,
		trustedProxies: trustedProxies,
		auth:           config.Auth,
		proxy:          config.Proxy,
		sessions:       config.Sessions,
		local:          config.Local,
		health:         health,
		static:         config.Static,
		logger:         logger,
	}, nil
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
	setSecurityHeaders(writer.Header())
	writer.Header().Set("X-Request-Id", requestID)
	startedAt := time.Now()
	recorder := &statusRecorder{ResponseWriter: writer, status: stdhttp.StatusOK}

	if !router.validAuthority(request) || !router.validOrigin(request.Header.Get("Origin")) {
		writeError(recorder, requestID, stdhttp.StatusForbidden, "ORIGIN_REJECTED", "请求来源不受信任", false)
		router.logRequest(request, recorder.status, startedAt, "ORIGIN_REJECTED")
		return
	}
	if request.Method == stdhttp.MethodOptions {
		writeError(recorder, requestID, stdhttp.StatusForbidden, "ORIGIN_REJECTED", "不接受跨域预检请求", false)
		router.logRequest(request, recorder.status, startedAt, "ORIGIN_REJECTED")
		return
	}
	if isMutation(request.Method) && request.Header.Get("Origin") == "" {
		writeError(recorder, requestID, stdhttp.StatusForbidden, "ORIGIN_REJECTED", "请求来源不受信任", false)
		router.logRequest(request, recorder.status, startedAt, "ORIGIN_REJECTED")
		return
	}

	request, err := router.withOptionalSession(request)
	if err != nil {
		status := stdhttp.StatusInternalServerError
		code := "LOCAL_STORAGE_ERROR"
		message := "本地会话存储不可用"
		retryable := true
		if errors.Is(err, errInvalidTimezone) {
			status = stdhttp.StatusBadRequest
			code = "VALIDATION_ERROR"
			message = "时区无效"
			retryable = false
		}
		writeError(recorder, requestID, status, code, message, retryable)
		router.logRequest(request, recorder.status, startedAt, code)
		return
	}
	errorCode := router.dispatch(recorder, request, requestID)
	router.logRequest(request, recorder.status, startedAt, errorCode)
}

func (router *router) dispatch(writer stdhttp.ResponseWriter, request *stdhttp.Request, requestID string) string {
	path := request.URL.Path
	switch {
	case request.Method == stdhttp.MethodGet && (path == "/api/healthz" || path == "/api/readyz"):
		router.health.ServeHTTP(writer, withPath(request, strings.TrimPrefix(path, "/api")))
		return ""
	case request.Method == stdhttp.MethodGet && path == "/api/auth/folo/providers" && router.auth != nil:
		router.auth.Providers(writer, sanitizeBrowserCredentials(request))
		return ""
	case request.Method == stdhttp.MethodPost && path == "/api/auth/folo/social-start" && router.auth != nil:
		router.auth.SocialStart(writer, sanitizeBrowserCredentials(request))
		return ""
	case request.Method == stdhttp.MethodPost && path == "/api/auth/folo/token" && router.auth != nil:
		router.auth.Token(writer, sanitizeBrowserCredentials(request))
		return ""
	case request.Method == stdhttp.MethodPost && path == "/api/auth/folo/email" && router.auth != nil:
		router.auth.Email(writer, sanitizeBrowserCredentials(request))
		return ""
	case request.Method == stdhttp.MethodPost && path == "/api/auth/folo/two-factor" && router.auth != nil:
		router.auth.TwoFactor(writer, sanitizeBrowserCredentials(request))
		return ""
	case request.Method == stdhttp.MethodPost && path == "/api/auth/logout" && router.auth != nil:
		if _, ok := session.FromContext(request.Context()); ok && !validCSRF(request) {
			writeError(writer, requestID, stdhttp.StatusForbidden, "CSRF_INVALID", "安全令牌无效，请刷新后重试", false)
			return "CSRF_INVALID"
		}
		router.auth.Logout(writer, sanitizeBrowserCredentials(request))
		return ""
	case request.Method == stdhttp.MethodGet && path == "/api/tantan/v1/session" && router.auth != nil:
		router.auth.CurrentSession(writer, sanitizeBrowserCredentials(request))
		return ""
	case strings.HasPrefix(path, localAPIPrefix):
		return router.dispatchLocal(writer, request, requestID)
	case path == foloAPIPrefix || strings.HasPrefix(path, foloAPIPrefix+"/"):
		return router.dispatchFolo(writer, request, requestID)
	case strings.HasPrefix(path, "/api/") || path == "/api":
		writeError(writer, requestID, stdhttp.StatusNotFound, "NOT_FOUND", "接口不存在", false)
		return "NOT_FOUND"
	case (request.Method == stdhttp.MethodGet || request.Method == stdhttp.MethodHead) && router.static != nil:
		router.static.ServeHTTP(writer, sanitizeBrowserCredentials(request))
		return ""
	default:
		writeError(writer, requestID, stdhttp.StatusNotFound, "NOT_FOUND", "页面不存在", false)
		return "NOT_FOUND"
	}
}

func (router *router) dispatchLocal(writer stdhttp.ResponseWriter, request *stdhttp.Request, requestID string) string {
	if _, ok := session.FromContext(request.Context()); !ok {
		writeError(writer, requestID, stdhttp.StatusUnauthorized, "AUTH_REQUIRED", "请先登录", false)
		return "AUTH_REQUIRED"
	}
	if isMutation(request.Method) && !validCSRF(request) {
		writeError(writer, requestID, stdhttp.StatusForbidden, "CSRF_INVALID", "安全令牌无效，请刷新后重试", false)
		return "CSRF_INVALID"
	}
	if router.local == nil {
		writeError(writer, requestID, stdhttp.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未准备就绪", true)
		return "SERVICE_NOT_READY"
	}
	localRequest := withPath(sanitizeBrowserCredentials(request), strings.TrimPrefix(request.URL.Path, "/api"))
	router.local.ServeHTTP(writer, localRequest)
	return ""
}

func (router *router) dispatchFolo(writer stdhttp.ResponseWriter, request *stdhttp.Request, requestID string) string {
	if router.proxy == nil {
		writeError(writer, requestID, stdhttp.StatusServiceUnavailable, "SERVICE_NOT_READY", "服务尚未准备就绪", true)
		return "SERVICE_NOT_READY"
	}
	upstreamPath := strings.TrimPrefix(request.URL.Path, foloAPIPrefix)
	if upstreamPath == "" {
		upstreamPath = "/"
	}
	proxyRequest := withPath(sanitizeBrowserCredentials(request), upstreamPath)
	decision := router.proxy.Decide(request.Method, proxyRequest.URL.EscapedPath())
	if decision.Kind == folo.DecisionAllow {
		if _, ok := session.FromContext(request.Context()); !ok {
			writeError(writer, requestID, stdhttp.StatusUnauthorized, "AUTH_REQUIRED", "请先登录", false)
			return "AUTH_REQUIRED"
		}
		if decision.Mutation && !validCSRF(request) {
			writeError(writer, requestID, stdhttp.StatusForbidden, "CSRF_INVALID", "安全令牌无效，请刷新后重试", false)
			return "CSRF_INVALID"
		}
	}
	router.proxy.ServeHTTP(writer, proxyRequest)
	return ""
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

func (router *router) validAuthority(request *stdhttp.Request) bool {
	if headerBytes(request.Header) > maximumHeaderSize {
		return false
	}
	forwarded := hasForwardedHeaders(request.Header)
	trusted := router.isTrustedProxy(request.RemoteAddr)
	if forwarded && !trusted {
		return false
	}
	if !forwarded {
		return request.Host == router.publicOrigin.Host
	}
	if request.Header.Get("Forwarded") != "" || request.Header.Get("X-Forwarded-For") == "" {
		return false
	}
	host, ok := oneHeaderValue(request.Header.Get("X-Forwarded-Host"))
	if !ok || host != router.publicOrigin.Host {
		return false
	}
	protocol, ok := oneHeaderValue(request.Header.Get("X-Forwarded-Proto"))
	return ok && strings.EqualFold(protocol, router.publicOrigin.Scheme)
}

func (router *router) validOrigin(origin string) bool {
	return origin == "" || origin == router.publicOrigin.String()
}

func (router *router) isTrustedProxy(remoteAddress string) bool {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err != nil {
		host = remoteAddress
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, network := range router.trustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
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
	return sanitized
}

func withPath(request *stdhttp.Request, path string) *stdhttp.Request {
	clone := request.Clone(request.Context())
	urlCopy := *request.URL
	urlCopy.Path = path
	urlCopy.RawPath = ""
	clone.URL = &urlCopy
	return clone
}

func validCSRF(request *stdhttp.Request) bool {
	record, ok := session.FromContext(request.Context())
	return ok && session.ValidCSRF(record, request.Header.Get("X-CSRF-Token"))
}

func parsePublicOrigin(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		raw = defaultOrigin
	}
	value, err := url.Parse(raw)
	if err != nil || value.User != nil || value.Host == "" || value.RawQuery != "" || value.Fragment != "" || (value.Path != "" && value.Path != "/") {
		return nil, errors.New("public origin is invalid")
	}
	value.Scheme = strings.ToLower(value.Scheme)
	if value.Scheme != "https" && !(value.Scheme == "http" && isLoopbackHost(value.Hostname())) {
		return nil, errors.New("public origin must use HTTPS or loopback HTTP")
	}
	value.Path = ""
	value.RawPath = ""
	return value, nil
}

func parseTrustedProxyCIDRs(values []string) ([]*net.IPNet, error) {
	result := make([]*net.IPNet, 0, len(values))
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return nil, errors.New("trusted proxy CIDR is invalid")
		}
		_, network, err := net.ParseCIDR(raw)
		if err != nil {
			return nil, errors.New("trusted proxy CIDR is invalid")
		}
		result = append(result, network)
	}
	return result, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func hasForwardedHeaders(header stdhttp.Header) bool {
	for _, name := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-Ip"} {
		if header.Get(name) != "" {
			return true
		}
	}
	return false
}

func oneHeaderValue(value string) (string, bool) {
	value = strings.TrimSpace(value)
	return value, value != "" && !strings.ContainsAny(value, ",\r\n")
}

func headerBytes(header stdhttp.Header) int {
	size := 0
	for name, values := range header {
		size += len(name)
		for _, value := range values {
			size += len(value)
		}
	}
	return size
}

func routeLabel(path string) string {
	switch path {
	case "/api/healthz", "/api/readyz", "/api/auth/folo/providers", "/api/auth/folo/social-start", "/api/auth/folo/token", "/api/auth/folo/email", "/api/auth/folo/two-factor", "/api/auth/logout", "/api/tantan/v1/session":
		return path
	}
	if strings.HasPrefix(path, localAPIPrefix) {
		return "/api/tantan/v1/*"
	}
	if path == foloAPIPrefix || strings.HasPrefix(path, foloAPIPrefix+"/") {
		return "/api/folo/*"
	}
	if strings.HasPrefix(path, "/api") {
		return "/api/*"
	}
	return "static"
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

func isMutation(method string) bool {
	return method == stdhttp.MethodPost || method == stdhttp.MethodPut || method == stdhttp.MethodPatch || method == stdhttp.MethodDelete
}

func newRequestID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err == nil {
		return hex.EncodeToString(buffer)
	}
	return "local-request-id"
}

func setSecurityHeaders(header stdhttp.Header) {
	header.Set("Cache-Control", "no-store")
	header.Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
}

func writeJSON(writer stdhttp.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer stdhttp.ResponseWriter, requestID string, status int, code, message string, retryable bool) {
	writeJSON(writer, status, map[string]any{
		"requestId": requestID,
		"error": map[string]any{
			"code":      code,
			"message":   message,
			"retryable": retryable,
		},
	})
}
