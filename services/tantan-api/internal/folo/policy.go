package folo

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

//go:embed route-policy.json
var routePolicyJSON []byte

type DecisionKind uint8

const (
	DecisionDenied DecisionKind = iota
	DecisionAllow
	DecisionRemoved
)

type Decision struct {
	Kind             DecisionKind
	RouteID          string
	Status           int
	Code             string
	Mutation         bool
	Path             string
	MaxRequestBytes  int64
	MaxResponseBytes int64
}

type routeDefinition struct {
	ID                 string   `json:"id"`
	Methods            []string `json:"methods"`
	PathPattern        string   `json:"pathPattern"`
	Mutation           bool     `json:"mutation"`
	MutationMethods    []string `json:"mutationMethods"`
	MaxRequestBytes    int64    `json:"maxRequestBytes"`
	MaxResponseBytes   int64    `json:"maxResponseBytes"`
	Status             int      `json:"status"`
	Code               string   `json:"code"`
	compiledExpression *regexp.Regexp
}

type policyDocument struct {
	SchemaVersion      string            `json:"schemaVersion"`
	SDKVersion         string            `json:"sdkVersion"`
	DefaultAction      string            `json:"defaultAction"`
	DefaultDenyStatus  int               `json:"defaultDenyStatus"`
	DefaultDenyCode    string            `json:"defaultDenyCode"`
	PublicPrefix       string            `json:"publicPrefix"`
	InternalAuthRoutes []routeDefinition `json:"internalAuthRoutes"`
	Enabled            []routeDefinition `json:"enabled"`
	DisabledByDefault  []routeDefinition `json:"disabledByDefault"`
	Removed            []routeDefinition `json:"removed"`
	Upstream           struct {
		MaxDefaultRequestBytes      int64 `json:"maxDefaultRequestBytes"`
		MaxDefaultResponseBytes     int64 `json:"maxDefaultResponseBytes"`
		MaxEntryStreamResponseBytes int64 `json:"maxEntryStreamResponseBytes"`
	} `json:"upstream"`
}

type Policy struct {
	document policyDocument
}

func LoadPolicy() (*Policy, error) {
	var document policyDocument
	if err := json.Unmarshal(routePolicyJSON, &document); err != nil {
		return nil, fmt.Errorf("decode embedded Folo route policy: %w", err)
	}
	if document.SchemaVersion != "2.0.0" || document.SDKVersion != "0.3.95" || document.PublicPrefix != "/api/folo" {
		return nil, errors.New("unsupported embedded Folo route policy version")
	}
	if document.DefaultAction != "deny" || document.DefaultDenyStatus != http.StatusForbidden || document.DefaultDenyCode != "FOLO_ROUTE_DENIED" {
		return nil, errors.New("embedded Folo route policy is not deny by default")
	}
	for _, routes := range [][]routeDefinition{document.InternalAuthRoutes, document.Enabled, document.DisabledByDefault, document.Removed} {
		for index := range routes {
			if !strings.HasPrefix(routes[index].PathPattern, "^") {
				return nil, fmt.Errorf("route pattern must be anchored at its start: %s", routes[index].PathPattern)
			}
			expression, err := regexp.Compile(routes[index].PathPattern)
			if err != nil {
				return nil, fmt.Errorf("compile route pattern %s: %w", routes[index].PathPattern, err)
			}
			routes[index].compiledExpression = expression
		}
	}
	return &Policy{document: document}, nil
}

func (policy *Policy) Decide(method, escapedPath string) Decision {
	path, err := url.PathUnescape(escapedPath)
	if err != nil || path == "" || path[0] != '/' || strings.ContainsRune(path, '\\') {
		return policy.denied(path)
	}
	method = strings.ToUpper(method)

	for _, route := range policy.document.Removed {
		if route.compiledExpression.MatchString(path) && methodAllowed(route.Methods, method) {
			return Decision{
				Kind:             DecisionRemoved,
				RouteID:          routeLabel(route, "removed"),
				Status:           route.Status,
				Code:             route.Code,
				Path:             path,
				MaxRequestBytes:  policy.document.Upstream.MaxDefaultRequestBytes,
				MaxResponseBytes: policy.document.Upstream.MaxDefaultResponseBytes,
			}
		}
	}

	for _, route := range policy.document.Enabled {
		if route.compiledExpression.MatchString(path) && slices.Contains(route.Methods, method) {
			maxRequestBytes := route.MaxRequestBytes
			if maxRequestBytes == 0 {
				maxRequestBytes = policy.document.Upstream.MaxDefaultRequestBytes
			}
			maxResponseBytes := route.MaxResponseBytes
			if maxResponseBytes == 0 {
				maxResponseBytes = policy.document.Upstream.MaxDefaultResponseBytes
			}
			return Decision{
				Kind:             DecisionAllow,
				RouteID:          routeLabel(route, "allowed"),
				Mutation:         route.Mutation || slices.Contains(route.MutationMethods, method),
				Path:             path,
				MaxRequestBytes:  maxRequestBytes,
				MaxResponseBytes: maxResponseBytes,
			}
		}
	}

	return policy.denied(path)
}

func methodAllowed(methods []string, method string) bool {
	return len(methods) == 0 || slices.Contains(methods, method)
}

func (policy *Policy) denied(path string) Decision {
	return Decision{
		Kind:             DecisionDenied,
		RouteID:          "default-deny",
		Status:           policy.document.DefaultDenyStatus,
		Code:             policy.document.DefaultDenyCode,
		Path:             path,
		MaxRequestBytes:  policy.document.Upstream.MaxDefaultRequestBytes,
		MaxResponseBytes: policy.document.Upstream.MaxDefaultResponseBytes,
	}
}

func routeLabel(route routeDefinition, fallback string) string {
	if route.ID != "" {
		return route.ID
	}
	return fallback
}
