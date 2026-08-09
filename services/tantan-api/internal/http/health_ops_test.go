package http_test

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	localhttp "tantan.local/tantan-api/internal/http"
	"tantan.local/tantan-api/internal/ops"
)

type readinessStub struct{ result ops.ReadinessResult }

func (stub readinessStub) Check(context.Context) ops.ReadinessResult { return stub.result }

func TestHealthHandlerReturnsContractAndFailsReadinessClosed(t *testing.T) {
	ready := ops.ReadinessResult{Ready: true, Checks: ops.ReadinessChecks{SQLite: "ok", Migrations: "ok", Keychain: "ok"}}
	handler := localhttp.NewHealthHandler("test-version", readinessStub{result: ready})
	healthRequest := httptest.NewRequest(stdhttp.MethodGet, "http://127.0.0.1:3000/healthz", nil)
	healthResponse := httptest.NewRecorder()
	handler.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != stdhttp.StatusOK || healthResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("health status=%d headers=%v", healthResponse.Code, healthResponse.Header())
	}
	var health map[string]any
	if err := json.Unmarshal(healthResponse.Body.Bytes(), &health); err != nil || health["status"] != "ok" || health["version"] != "test-version" {
		t.Fatalf("health=%v err=%v", health, err)
	}
	readyRequest := httptest.NewRequest(stdhttp.MethodGet, "http://127.0.0.1:3000/readyz", nil)
	readyResponse := httptest.NewRecorder()
	handler.ServeHTTP(readyResponse, readyRequest)
	if readyResponse.Code != stdhttp.StatusOK {
		t.Fatalf("ready status=%d body=%s", readyResponse.Code, readyResponse.Body.String())
	}

	notReady := ops.ReadinessResult{Ready: false, Checks: ops.ReadinessChecks{SQLite: "error", Migrations: "error", Keychain: "ok"}}
	handler = localhttp.NewHealthHandler("test-version", readinessStub{result: notReady})
	failedResponse := httptest.NewRecorder()
	handler.ServeHTTP(failedResponse, readyRequest)
	if failedResponse.Code != stdhttp.StatusServiceUnavailable || !stringsContain(failedResponse.Body.String(), "SERVICE_NOT_READY") {
		t.Fatalf("failed readiness status=%d body=%s", failedResponse.Code, failedResponse.Body.String())
	}
}

func stringsContain(value, part string) bool {
	for index := 0; index+len(part) <= len(value); index++ {
		if value[index:index+len(part)] == part {
			return true
		}
	}
	return false
}
