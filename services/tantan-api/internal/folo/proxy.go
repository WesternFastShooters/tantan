package folo

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	"tantan.local/tantan-api/internal/session"
)

var allowedOrigins = map[string]struct{}{
	"http://127.0.0.1:3000": {},
	"http://127.0.0.1:5173": {},
	"http://localhost:3000": {},
	"http://localhost:5173": {},
}

var strippedRequestHeaders = map[string]struct{}{
	"Authorization":       {},
	"Connection":          {},
	"Content-Length":      {},
	"Cookie":              {},
	"Expect":              {},
	"Forwarded":           {},
	"Keep-Alive":          {},
	"Proxy-Authorization": {},
	"Proxy-Connection":    {},
	"Referer":             {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
	"X-Forwarded-For":     {},
	"X-Forwarded-Host":    {},
	"X-Forwarded-Proto":   {},
	"X-Real-Ip":           {},
}

var strippedResponseHeaders = map[string]struct{}{
	"Access-Control-Allow-Credentials": {},
	"Access-Control-Allow-Headers":     {},
	"Access-Control-Allow-Methods":     {},
	"Access-Control-Allow-Origin":      {},
	"Connection":                       {},
	"Keep-Alive":                       {},
	"Proxy-Authenticate":               {},
	"Set-Cookie":                       {},
	"Te":                               {},
	"Trailer":                          {},
	"Transfer-Encoding":                {},
	"Upgrade":                          {},
}

type ProxyConfig struct {
	Policy   *Policy
	Upstream *url.URL
	Client   *http.Client
	Secrets  session.SecretStore
	Logger   *slog.Logger
}

type Proxy struct {
	policy      *Policy
	upstream    *url.URL
	client      *http.Client
	secrets     session.SecretStore
	logger      *slog.Logger
	deniedCount atomic.Uint64
}

func NewProxy(config ProxyConfig) (*Proxy, error) {
	if config.Policy == nil || config.Upstream == nil || config.Secrets == nil {
		return nil, errors.New("proxy policy, upstream and secret store are required")
	}
	if !isAllowedAPIUpstream(config.Upstream) {
		return nil, errors.New("Folo upstream must be https://api.folo.is or a loopback test server")
	}
	client := config.Client
	if client == nil {
		client = http.DefaultClient
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	upstreamCopy := *config.Upstream
	return &Proxy{
		policy:   config.Policy,
		upstream: &upstreamCopy,
		client:   &clientCopy,
		secrets:  config.Secrets,
		logger:   logger,
	}, nil
}

func (proxy *Proxy) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	escapedPath := request.URL.EscapedPath()
	decision := proxy.policy.Decide(request.Method, escapedPath)
	if decision.Kind != DecisionAllow {
		proxy.deniedCount.Add(1)
		writeAPIError(writer, request, decision.Status, decision.Code, publicMessage(decision.Code))
		proxy.logger.WarnContext(request.Context(), "folo_route_rejected",
			slog.String("method", request.Method),
			slog.String("route", decision.RouteID),
			slog.String("errorCode", decision.Code),
		)
		return
	}

	record, ok := session.FromContext(request.Context())
	if !ok {
		writeAPIError(writer, request, http.StatusUnauthorized, "AUTH_REQUIRED", "请先登录")
		return
	}
	if decision.Mutation && !isAllowedOrigin(request.Header.Get("Origin")) {
		writeAPIError(writer, request, http.StatusForbidden, "ORIGIN_REJECTED", "请求来源不受信任")
		return
	}
	upstreamToken, err := proxy.secrets.Get(request.Context(), record.IDHash)
	if err != nil || !validCookieValue(upstreamToken) {
		writeAPIError(writer, request, http.StatusUnauthorized, "AUTH_REQUIRED", "登录已失效，请重新登录")
		return
	}

	body, err := readBounded(request.Body, decision.MaxRequestBytes)
	if err != nil {
		writeAPIError(writer, request, http.StatusRequestEntityTooLarge, "VALIDATION_ERROR", "请求内容过大")
		return
	}

	upstreamURL := *proxy.upstream
	upstreamURL.Path = decision.Path
	upstreamURL.RawPath = ""
	upstreamURL.RawQuery = request.URL.RawQuery
	upstreamRequest, err := http.NewRequestWithContext(request.Context(), request.Method, upstreamURL.String(), bytes.NewReader(body))
	if err != nil {
		writeAPIError(writer, request, http.StatusBadGateway, "FOLO_UNAVAILABLE", "暂时无法连接 Folo")
		return
	}
	copyAllowedHeaders(upstreamRequest.Header, request.Header, strippedRequestHeaders)
	upstreamRequest.Header.Set("Cookie", "__Secure-better-auth.session_token="+upstreamToken)

	response, err := proxy.client.Do(upstreamRequest)
	if err != nil {
		proxy.logger.WarnContext(request.Context(), "folo_upstream_failed",
			slog.String("method", request.Method),
			slog.String("route", decision.RouteID),
			slog.String("errorCode", "FOLO_UNAVAILABLE"),
		)
		writeAPIError(writer, request, http.StatusBadGateway, "FOLO_UNAVAILABLE", "暂时无法连接 Folo")
		return
	}
	defer response.Body.Close()
	responseBody, err := readBounded(response.Body, decision.MaxResponseBytes)
	if err != nil {
		writeAPIError(writer, request, http.StatusBadGateway, "FOLO_UNAVAILABLE", "Folo 响应超出安全限制")
		return
	}

	copyAllowedHeaders(writer.Header(), response.Header, strippedResponseHeaders)
	writer.WriteHeader(response.StatusCode)
	_, _ = writer.Write(responseBody)
	proxy.logger.InfoContext(request.Context(), "folo_proxy_request",
		slog.String("method", request.Method),
		slog.String("route", decision.RouteID),
		slog.Int("status", response.StatusCode),
	)
}

func (proxy *Proxy) DeniedCount() uint64 {
	return proxy.deniedCount.Load()
}

func (proxy *Proxy) Decide(method, escapedPath string) Decision {
	return proxy.policy.Decide(method, escapedPath)
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	if reader == nil {
		return nil, nil
	}
	limited := io.LimitReader(reader, limit+1)
	contents, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(contents)) > limit {
		return nil, errors.New("size limit exceeded")
	}
	return contents, nil
}

func copyAllowedHeaders(target, source http.Header, stripped map[string]struct{}) {
	connectionHeaders := make(map[string]struct{})
	for _, value := range source.Values("Connection") {
		for _, name := range strings.Split(value, ",") {
			connectionHeaders[http.CanonicalHeaderKey(strings.TrimSpace(name))] = struct{}{}
		}
	}
	for name, values := range source {
		canonicalName := http.CanonicalHeaderKey(name)
		if _, blocked := stripped[canonicalName]; blocked {
			continue
		}
		if _, blocked := connectionHeaders[canonicalName]; blocked {
			continue
		}
		for _, value := range values {
			target.Add(canonicalName, value)
		}
	}
}

func isAllowedOrigin(origin string) bool {
	_, ok := allowedOrigins[origin]
	return ok
}

func isLoopbackTestURL(value *url.URL) bool {
	return value.Scheme == "http" && (value.Hostname() == "127.0.0.1" || value.Hostname() == "localhost")
}

func isAllowedAPIUpstream(value *url.URL) bool {
	if value == nil || value.User != nil || value.RawQuery != "" || value.Fragment != "" || (value.Path != "" && value.Path != "/") {
		return false
	}
	return (value.Scheme == "https" && value.Host == "api.folo.is") || isLoopbackTestURL(value)
}

func writeAPIError(writer http.ResponseWriter, request *http.Request, status int, code, message string) {
	requestID := request.Header.Get("X-Request-Id")
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

func publicMessage(code string) string {
	if code == "FOLO_FEATURE_REMOVED" {
		return "该 Folo 功能已从 Tantan 移除"
	}
	return "该 Folo 路由未获允许"
}
