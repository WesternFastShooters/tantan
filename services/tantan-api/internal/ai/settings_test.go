package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/ai"
)

const aiKeyCanary = "test-DO-NOT-STORE-BE-AI-TOPIC"

type memorySecrets struct {
	values   map[string]string
	getError error
}

func (secrets *memorySecrets) Get(_ context.Context, account string) (string, error) {
	if secrets.getError != nil {
		return "", secrets.getError
	}
	value, ok := secrets.values[account]
	if !ok {
		return "", ai.ErrSecretNotFound
	}
	return value, nil
}

func (*memorySecrets) Set(context.Context, string, string) error {
	return errors.New("server AI configuration is read-only")
}

func (*memorySecrets) Delete(context.Context, string) error {
	return errors.New("server AI configuration is read-only")
}

func TestAISettingsAreFixedAndReadOnly(t *testing.T) {
	secrets := &memorySecrets{values: map[string]string{ai.FixedProviderID: aiKeyCanary}}
	service, err := ai.NewSettingsService(ai.SettingsConfig{Secrets: secrets})
	if err != nil {
		t.Fatalf("create settings: %v", err)
	}
	response, err := service.Get(context.Background())
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), aiKeyCanary) {
		t.Fatal("AI credential was returned by the public settings response")
	}
	if !response.Configured || !response.HasAPIKey || response.ProviderID != ai.FixedProviderID || response.Model != ai.FixedModel || response.BaseURL != ai.FixedBaseURL {
		t.Fatalf("public settings=%s", encoded)
	}
	if !regexp.MustCompile(`^[0-9A-F]{8}$`).MatchString(response.KeyFingerprint) {
		t.Fatalf("key fingerprint=%q", response.KeyFingerprint)
	}

	active, key, err := service.Credential(context.Background(), ai.DefaultPromptVersion)
	if err != nil || key != aiKeyCanary {
		t.Fatalf("credential active=%#v err=%v", active, err)
	}
	if active.ProviderID != ai.FixedProviderID || active.Model != ai.FixedModel || active.BaseURL != ai.FixedBaseURL || len(active.Fingerprint) != 12 {
		t.Fatalf("active provider=%#v", active)
	}
}

func TestAISettingsRemainReadyWithoutServerKey(t *testing.T) {
	service, err := ai.NewSettingsService(ai.SettingsConfig{Secrets: &memorySecrets{values: map[string]string{}}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := service.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if response.Configured || response.HasAPIKey || response.KeyFingerprint != "" || response.ProviderID != ai.FixedProviderID || response.Model != ai.FixedModel || response.BaseURL != ai.FixedBaseURL {
		t.Fatalf("settings=%#v", response)
	}
	if _, _, err := service.Credential(context.Background(), ai.DefaultPromptVersion); !errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("credential error=%v", err)
	}
}

func TestAISettingsSecretFailuresAreRedacted(t *testing.T) {
	service, err := ai.NewSettingsService(ai.SettingsConfig{Secrets: &memorySecrets{values: map[string]string{}, getError: errors.New("backend-" + aiKeyCanary)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(context.Background()); err == nil || strings.Contains(err.Error(), aiKeyCanary) {
		t.Fatalf("settings error=%v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestProviderUsesOnlyFixedGeminiOpenAIEndpoint(t *testing.T) {
	var observedURL string
	var observedAuthorization string
	var observedBody map[string]any
	client, err := ai.NewProviderClient(ai.ProviderClientConfig{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		observedURL = request.URL.String()
		observedAuthorization = request.Header.Get("Authorization")
		if err := json.NewDecoder(request.Body).Decode(&observedBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{\"version\":1}"}}]}`)),
			Request:    request,
		}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	output, err := client.Generate(context.Background(), aiKeyCanary, ai.GenerationRequest{SchemaName: ai.EnrichmentSchemaName, SystemPrompt: "Return JSON", UserPrompt: "fixture"})
	if err != nil || string(output) != `{"version":1}` {
		t.Fatalf("output=%s err=%v", output, err)
	}
	if observedURL != ai.FixedBaseURL+"/chat/completions" || observedAuthorization != "Bearer "+aiKeyCanary || strings.Contains(observedURL, aiKeyCanary) {
		t.Fatalf("url=%q authorization=%q", observedURL, observedAuthorization)
	}
	if observedBody["model"] != ai.FixedModel {
		t.Fatalf("model=%v", observedBody["model"])
	}
	for _, deprecated := range []string{"temperature", "top_p", "top_k"} {
		if _, exists := observedBody[deprecated]; exists {
			t.Fatalf("deprecated sampling field %q was sent", deprecated)
		}
	}
	responseFormat, ok := observedBody["response_format"].(map[string]any)
	if !ok || responseFormat["type"] != "json_schema" {
		t.Fatalf("response_format=%#v", observedBody["response_format"])
	}
	encoded, _ := json.Marshal(observedBody)
	if strings.Contains(string(encoded), aiKeyCanary) {
		t.Fatal("credential leaked into request body")
	}
	if _, err := ai.ProviderPreset("google"); err == nil {
		t.Fatal("legacy provider was accepted")
	}
	if _, err := ai.ProviderPreset("custom"); err == nil {
		t.Fatal("custom provider was accepted")
	}
	if err := ai.ValidateModel("gemini-custom"); err == nil {
		t.Fatal("custom model was accepted")
	}
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "fc00::1", "0.0.0.0"} {
		if err := ai.ValidateDialIP(net.ParseIP(address)); err == nil {
			t.Fatalf("unsafe provider address %s was accepted", address)
		}
	}
}

func TestProviderRejectsUnknownSchemaBeforeNetwork(t *testing.T) {
	called := false
	client, err := ai.NewProviderClient(ai.ProviderClientConfig{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("unexpected network")
	})})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Generate(context.Background(), aiKeyCanary, ai.GenerationRequest{SchemaName: "user-controlled-schema", SystemPrompt: "system", UserPrompt: "user"})
	if err == nil || called {
		t.Fatalf("error=%v called=%t", err, called)
	}
}

func TestProviderFailureDoesNotReflectSecretOrResponseBody(t *testing.T) {
	client, err := ai.NewProviderClient(ai.ProviderClientConfig{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusInternalServerError, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("private-provider-body-" + aiKeyCanary)), Request: request}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Generate(context.Background(), aiKeyCanary, ai.GenerationRequest{SchemaName: ai.EnrichmentSchemaName, SystemPrompt: "system", UserPrompt: "user"})
	if err == nil || strings.Contains(err.Error(), aiKeyCanary) || strings.Contains(err.Error(), "private-provider-body") {
		t.Fatalf("provider error=%v", err)
	}
}

func TestProviderConnectionTestUsesFixedModelWithoutPersistence(t *testing.T) {
	start := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	clockCalls := 0
	clock := func() time.Time {
		clockCalls++
		return start.Add(time.Duration(clockCalls-1) * 125 * time.Millisecond)
	}
	seenKey := ""
	result, err := ai.TestConnection(context.Background(), aiKeyCanary, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		seenKey = strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`)), Request: request}, nil
	}), clock)
	if err != nil || !result.OK || result.LatencyMS != 125 || result.Model != ai.FixedModel || seenKey != aiKeyCanary {
		t.Fatalf("result=%#v seenKey=%q err=%v", result, seenKey, err)
	}
}

func TestProviderRetryClassificationOnlyRetriesTransientFailures(t *testing.T) {
	for _, test := range []struct {
		status    int
		retryable bool
	}{
		{status: http.StatusBadRequest, retryable: false},
		{status: http.StatusUnauthorized, retryable: false},
		{status: http.StatusTooManyRequests, retryable: true},
		{status: http.StatusInternalServerError, retryable: true},
	} {
		client, err := ai.NewProviderClient(ai.ProviderClientConfig{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: test.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("redacted")), Request: request}, nil
		})})
		if err != nil {
			t.Fatal(err)
		}
		_, failure := client.Generate(context.Background(), aiKeyCanary, ai.GenerationRequest{SchemaName: ai.EnrichmentSchemaName, SystemPrompt: "system", UserPrompt: "user"})
		if failure == nil || ai.ShouldRetryProvider(failure) != test.retryable {
			t.Fatalf("status=%d retryable=%t err=%v", test.status, ai.ShouldRetryProvider(failure), failure)
		}
	}
}
