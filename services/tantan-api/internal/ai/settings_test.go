package ai_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/ai"
	"tantan.local/tantan-api/internal/storage"
)

const aiKeyCanary = "sk-test-DO-NOT-STORE-BE-AI-TOPIC"

type memorySecrets struct {
	values    map[string]string
	setError  error
	deleteErr error
}

func (secrets *memorySecrets) Get(_ context.Context, account string) (string, error) {
	value, ok := secrets.values[account]
	if !ok {
		return "", ai.ErrSecretNotFound
	}
	return value, nil
}

func (secrets *memorySecrets) Set(_ context.Context, account, value string) error {
	if secrets.setError != nil {
		return secrets.setError
	}
	secrets.values[account] = value
	return nil
}

func (secrets *memorySecrets) Delete(_ context.Context, account string) error {
	if secrets.deleteErr != nil {
		return secrets.deleteErr
	}
	delete(secrets.values, account)
	return nil
}

func openAIStore(t *testing.T) (*storage.Store, string) {
	t.Helper()
	directory := t.TempDir()
	store, err := storage.Open(context.Background(), storage.Config{DataDir: directory})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	return store, directory
}

func TestAISettingsPersistKeyOnlyInKeychainAndNeverReturnIt(t *testing.T) {
	ctx := context.Background()
	store, directory := openAIStore(t)
	secrets := &memorySecrets{values: map[string]string{}}
	service, err := ai.NewSettingsService(ai.SettingsConfig{Store: store, Secrets: secrets})
	if err != nil {
		t.Fatalf("create settings: %v", err)
	}
	response, err := service.Put(ctx, ai.ProviderInput{ProviderID: "openai", Model: "gpt-test", APIKey: aiKeyCanary})
	if err != nil {
		t.Fatalf("save settings: %v", err)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), aiKeyCanary) || !response.Configured || !response.HasAPIKey || response.BaseURL != "https://api.openai.com/v1" || len(response.KeyFingerprint) != 12 {
		t.Fatalf("public settings=%s", encoded)
	}
	if secrets.values["openai"] != aiKeyCanary {
		t.Fatal("API key was not stored in the Keychain abstraction")
	}
	got, err := service.Get(ctx)
	if err != nil || got != response {
		t.Fatalf("get settings=%#v err=%v", got, err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close storage: %v", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		contents, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(contents, []byte(aiKeyCanary)) {
			t.Fatalf("API key leaked to SQLite file %s", entry.Name())
		}
	}
}

func TestAISettingsKeychainFailureDoesNotChangeSQLiteAndDeleteIsKeyFirst(t *testing.T) {
	ctx := context.Background()
	store, _ := openAIStore(t)
	defer store.Close()
	secrets := &memorySecrets{values: map[string]string{}, setError: errors.New("keychain unavailable")}
	service, err := ai.NewSettingsService(ai.SettingsConfig{Store: store, Secrets: secrets})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Put(ctx, ai.ProviderInput{ProviderID: "openai", Model: "gpt-test", APIKey: aiKeyCanary}); err == nil || strings.Contains(err.Error(), aiKeyCanary) {
		t.Fatalf("save error=%v", err)
	}
	var configs int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_provider_configs").Scan(&configs); err != nil || configs != 0 {
		t.Fatalf("configs=%d err=%v", configs, err)
	}
	secrets.setError = nil
	if _, err := service.Put(ctx, ai.ProviderInput{ProviderID: "openai", Model: "gpt-test", APIKey: aiKeyCanary}); err != nil {
		t.Fatal(err)
	}
	secrets.deleteErr = errors.New("keychain delete unavailable")
	if err := service.Delete(ctx); err == nil {
		t.Fatal("delete unexpectedly succeeded")
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_provider_configs WHERE enabled=1").Scan(&configs); err != nil || configs != 1 {
		t.Fatalf("config changed after key deletion failure: count=%d err=%v", configs, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestProviderUsesOnlyPresetEndpointAndCredentialHeader(t *testing.T) {
	var observedURL string
	var observedAuthorization string
	var observedBody string
	client, err := ai.NewProviderClient(ai.ProviderClientConfig{
		ProviderID: "openai",
		Model:      "gpt-test",
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			observedURL = request.URL.String()
			observedAuthorization = request.Header.Get("Authorization")
			contents, _ := io.ReadAll(request.Body)
			observedBody = string(contents)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{\"version\":1}"}}]}`)),
				Request:    request,
			}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := client.Generate(context.Background(), aiKeyCanary, ai.GenerationRequest{SystemPrompt: "Return JSON", UserPrompt: "fixture"})
	if err != nil || string(output) != `{"version":1}` {
		t.Fatalf("output=%s err=%v", output, err)
	}
	if observedURL != "https://api.openai.com/v1/chat/completions" || observedAuthorization != "Bearer "+aiKeyCanary || strings.Contains(observedURL, aiKeyCanary) || strings.Contains(observedBody, aiKeyCanary) {
		t.Fatalf("url=%q authorization=%q body=%q", observedURL, observedAuthorization, observedBody)
	}
	if _, err := ai.NewProviderClient(ai.ProviderClientConfig{ProviderID: "custom", Model: "model", Transport: http.DefaultTransport}); err == nil {
		t.Fatal("custom provider endpoint was accepted")
	}
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "fc00::1", "0.0.0.0"} {
		if err := ai.ValidateDialIP(net.ParseIP(address)); err == nil {
			t.Fatalf("unsafe provider address %s was accepted", address)
		}
	}
}

func TestProviderFailureDoesNotReflectSecretOrResponseBody(t *testing.T) {
	client, err := ai.NewProviderClient(ai.ProviderClientConfig{
		ProviderID: "openai",
		Model:      "gpt-test",
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusInternalServerError, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("private-provider-body-" + aiKeyCanary)), Request: request}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Generate(context.Background(), aiKeyCanary, ai.GenerationRequest{SystemPrompt: "system", UserPrompt: "user"})
	if err == nil || strings.Contains(err.Error(), aiKeyCanary) || strings.Contains(err.Error(), "private-provider-body") {
		t.Fatalf("provider error=%v", err)
	}
}

func TestAllProviderAdaptersUseLockedEndpointAndHeader(t *testing.T) {
	tests := []struct {
		providerID   string
		wantURL      string
		wantHeader   string
		responseBody string
	}{
		{providerID: "openai", wantURL: "https://api.openai.com/v1/chat/completions", wantHeader: "Authorization", responseBody: `{"choices":[{"message":{"content":"ok"}}]}`},
		{providerID: "anthropic", wantURL: "https://api.anthropic.com/v1/messages", wantHeader: "x-api-key", responseBody: `{"content":[{"type":"text","text":"ok"}]}`},
		{providerID: "google", wantURL: "https://generativelanguage.googleapis.com/v1beta/models/model-test:generateContent", wantHeader: "x-goog-api-key", responseBody: `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`},
		{providerID: "deepseek", wantURL: "https://api.deepseek.com/chat/completions", wantHeader: "Authorization", responseBody: `{"choices":[{"message":{"content":"ok"}}]}`},
		{providerID: "openrouter", wantURL: "https://openrouter.ai/api/v1/chat/completions", wantHeader: "Authorization", responseBody: `{"choices":[{"message":{"content":"ok"}}]}`},
	}
	for _, test := range tests {
		t.Run(test.providerID, func(t *testing.T) {
			client, err := ai.NewProviderClient(ai.ProviderClientConfig{
				ProviderID: test.providerID,
				Model:      "model-test",
				Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
					if request.URL.String() != test.wantURL {
						t.Fatalf("url=%q", request.URL.String())
					}
					if got := request.Header.Get(test.wantHeader); got != aiKeyCanary && got != "Bearer "+aiKeyCanary {
						t.Fatalf("credential header=%q", got)
					}
					if strings.Contains(request.URL.String(), aiKeyCanary) {
						t.Fatal("credential leaked into URL")
					}
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(test.responseBody)), Request: request}, nil
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			output, err := client.Generate(context.Background(), aiKeyCanary, ai.GenerationRequest{SystemPrompt: "system", UserPrompt: "user"})
			if err != nil || string(output) != "ok" {
				t.Fatalf("output=%q err=%v", output, err)
			}
		})
	}
}

func TestProviderConnectionTestUsesTemporaryKeyWithoutPersistence(t *testing.T) {
	start := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	clockCalls := 0
	clock := func() time.Time {
		clockCalls++
		return start.Add(time.Duration(clockCalls-1) * 125 * time.Millisecond)
	}
	seenKey := ""
	result, err := ai.TestConnection(context.Background(), ai.ProviderInput{ProviderID: "openai", Model: "model-test", APIKey: aiKeyCanary}, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		seenKey = strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`)), Request: request}, nil
	}), clock)
	if err != nil || !result.OK || result.LatencyMS != 125 || result.Model != "model-test" || seenKey != aiKeyCanary {
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
		client, err := ai.NewProviderClient(ai.ProviderClientConfig{
			ProviderID: "openai",
			Model:      "model-test",
			Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("redacted")), Request: request}, nil
			}),
		})
		if err != nil {
			t.Fatal(err)
		}
		_, failure := client.Generate(context.Background(), aiKeyCanary, ai.GenerationRequest{SystemPrompt: "system", UserPrompt: "user"})
		if failure == nil || ai.ShouldRetryProvider(failure) != test.retryable {
			t.Fatalf("status=%d retryable=%t err=%v", test.status, ai.ShouldRetryProvider(failure), failure)
		}
	}
}
