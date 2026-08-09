package folo_test

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"tantan.local/tantan-api/internal/folo"
)

func TestEmbeddedRoutePolicyMatchesApprovedMachineContract(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve policy test path")
	}
	packageDirectory := filepath.Dir(filename)
	actual, err := os.ReadFile(filepath.Join(packageDirectory, "route-policy.json"))
	if err != nil {
		t.Fatalf("read embedded policy: %v", err)
	}
	expected, err := os.ReadFile(filepath.Join(packageDirectory, "..", "..", "..", "..", "spec-package", "api", "folo-route-policy.json"))
	if err != nil {
		t.Fatalf("read approved policy: %v", err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatal("embedded Folo route policy differs from approved machine contract")
	}
}

func TestRoutePolicyIsExactAndDenyByDefault(t *testing.T) {
	policy, err := folo.LoadPolicy()
	if err != nil {
		t.Fatalf("load route policy: %v", err)
	}

	tests := []struct {
		name     string
		method   string
		path     string
		kind     folo.DecisionKind
		status   int
		mutation bool
	}{
		{name: "enabled read", method: http.MethodGet, path: "/entries", kind: folo.DecisionAllow, status: 0},
		{name: "enabled mutation", method: http.MethodPost, path: "/reads", kind: folo.DecisionAllow, status: 0, mutation: true},
		{name: "RSS subscription remains enabled", method: http.MethodGet, path: "/subscriptions", kind: folo.DecisionAllow, status: 0},
		{name: "Folo AI removed", method: http.MethodPost, path: "/ai/summary", kind: folo.DecisionRemoved, status: http.StatusGone},
		{name: "Stripe subscription removed", method: http.MethodGet, path: "/better-auth/subscription/list", kind: folo.DecisionRemoved, status: http.StatusGone},
		{name: "RSSHub paid use removed", method: http.MethodPost, path: "/rsshub/use", kind: folo.DecisionRemoved, status: http.StatusGone},
		{name: "RSSHub paid use wrong method denied", method: http.MethodGet, path: "/rsshub/use", kind: folo.DecisionDenied, status: http.StatusForbidden},
		{name: "AI settings not enabled", method: http.MethodPatch, path: "/settings/ai", kind: folo.DecisionDenied, status: http.StatusForbidden},
		{name: "unknown denied", method: http.MethodGet, path: "/unknown", kind: folo.DecisionDenied, status: http.StatusForbidden},
		{name: "method mismatch denied", method: http.MethodDelete, path: "/entries", kind: folo.DecisionDenied, status: http.StatusForbidden},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := policy.Decide(test.method, test.path)
			if decision.Kind != test.kind || decision.Status != test.status || decision.Mutation != test.mutation {
				t.Fatalf("decision = %#v", decision)
			}
		})
	}
}

func TestRoutePolicyDecodesPathExactlyOnce(t *testing.T) {
	policy, err := folo.LoadPolicy()
	if err != nil {
		t.Fatalf("load route policy: %v", err)
	}

	if decision := policy.Decide(http.MethodPost, "/%61i/summary"); decision.Kind != folo.DecisionRemoved {
		t.Fatalf("single encoded AI path was not removed: %#v", decision)
	}
	if decision := policy.Decide(http.MethodPost, "/%2561i/summary"); decision.Kind != folo.DecisionDenied {
		t.Fatalf("double encoded path should be denied after one decode: %#v", decision)
	}
}
