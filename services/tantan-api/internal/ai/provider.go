package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	providerTimeout          = 60 * time.Second
	maxProviderRequestBytes  = 2 * 1024 * 1024
	maxProviderResponseBytes = 4 * 1024 * 1024
)

var ErrProviderUnavailable = errors.New("AI provider unavailable")

type temporaryFailure interface {
	Temporary() bool
}

func ShouldRetryProvider(err error) bool {
	if err == nil {
		return false
	}
	var temporary temporaryFailure
	if errors.As(err, &temporary) {
		return temporary.Temporary()
	}
	return errors.Is(err, ErrProviderUnavailable) || errors.Is(err, context.DeadlineExceeded)
}

type GenerationRequest struct {
	SchemaName   string
	SystemPrompt string
	UserPrompt   string
	Repair       bool
}

type Generator interface {
	Generate(ctx context.Context, apiKey string, request GenerationRequest) ([]byte, error)
}

type ProviderClientConfig struct {
	ProviderID string
	Model      string
	Transport  http.RoundTripper
}

type ProviderClient struct {
	preset Preset
	model  string
	client *http.Client
}

type providerError struct {
	status    int
	temporary bool
}

func (failure providerError) Error() string {
	if failure.status == 0 {
		return "AI provider is unavailable"
	}
	return fmt.Sprintf("AI provider returned status %d", failure.status)
}

func (failure providerError) Unwrap() error {
	return ErrProviderUnavailable
}

func (failure providerError) Temporary() bool {
	return failure.temporary
}

func NewProviderClient(config ProviderClientConfig) (*ProviderClient, error) {
	preset, err := ProviderPreset(strings.TrimSpace(config.ProviderID))
	if err != nil {
		return nil, err
	}
	model := strings.TrimSpace(config.Model)
	if err := ValidateModel(model); err != nil {
		return nil, err
	}
	transport := config.Transport
	if transport == nil {
		transport, err = newProviderTransport()
		if err != nil {
			return nil, err
		}
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   providerTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &ProviderClient{preset: preset, model: model, client: client}, nil
}

func (client *ProviderClient) Generate(ctx context.Context, apiKey string, generation GenerationRequest) ([]byte, error) {
	if count := utf8.RuneCountInString(apiKey); count < 8 || count > 4096 || strings.ContainsAny(apiKey, "\r\n\x00") {
		return nil, errors.New("AI provider credential is invalid")
	}
	if strings.TrimSpace(generation.SystemPrompt) == "" || strings.TrimSpace(generation.UserPrompt) == "" {
		return nil, errors.New("AI generation prompts are required")
	}
	target, body, err := client.request(generation)
	if err != nil {
		return nil, err
	}
	if len(body) > maxProviderRequestBytes {
		return nil, errors.New("AI provider request exceeds the safe limit")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("create AI provider request failed")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	switch client.preset.Kind {
	case "anthropic":
		request.Header.Set("x-api-key", apiKey)
		request.Header.Set("anthropic-version", "2023-06-01")
	case "google":
		request.Header.Set("x-goog-api-key", apiKey)
	default:
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := client.client.Do(request)
	if err != nil {
		return nil, providerError{temporary: true}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
		return nil, providerError{status: response.StatusCode, temporary: response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500}
	}
	contents, err := io.ReadAll(io.LimitReader(response.Body, maxProviderResponseBytes+1))
	if err != nil || len(contents) > maxProviderResponseBytes {
		return nil, providerError{temporary: true}
	}
	output, err := client.response(contents)
	if err != nil {
		return nil, err
	}
	if len(output) == 0 || len(output) > maxProviderResponseBytes {
		return nil, errors.New("AI provider returned invalid content")
	}
	return output, nil
}

func (client *ProviderClient) request(generation GenerationRequest) (string, []byte, error) {
	systemPrompt := safePromptText(generation.SystemPrompt, 20_000)
	userPrompt := safePromptText(generation.UserPrompt, 200_000)
	var target string
	var payload any
	switch client.preset.Kind {
	case "anthropic":
		target = client.preset.BaseURL + "/v1/messages"
		payload = map[string]any{
			"model":       client.model,
			"max_tokens":  8192,
			"temperature": 0,
			"system":      systemPrompt,
			"messages":    []map[string]string{{"role": "user", "content": userPrompt}},
		}
	case "google":
		target = client.preset.BaseURL + "/v1beta/models/" + url.PathEscape(client.model) + ":generateContent"
		payload = map[string]any{
			"systemInstruction": map[string]any{"parts": []map[string]string{{"text": systemPrompt}}},
			"contents":          []map[string]any{{"role": "user", "parts": []map[string]string{{"text": userPrompt}}}},
			"generationConfig":  map[string]any{"temperature": 0, "responseMimeType": "application/json"},
		}
	default:
		target = strings.TrimSuffix(client.preset.BaseURL, "/") + "/chat/completions"
		payload = map[string]any{
			"model":           client.model,
			"temperature":     0,
			"response_format": map[string]string{"type": "json_object"},
			"messages": []map[string]string{
				{"role": "system", "content": systemPrompt},
				{"role": "user", "content": userPrompt},
			},
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", nil, errors.New("encode AI provider request failed")
	}
	return target, body, nil
}

func (client *ProviderClient) response(contents []byte) ([]byte, error) {
	switch client.preset.Kind {
	case "anthropic":
		var payload struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(contents, &payload); err != nil {
			return nil, errors.New("AI provider returned invalid JSON")
		}
		for _, item := range payload.Content {
			if item.Type == "text" && item.Text != "" {
				return []byte(item.Text), nil
			}
		}
	case "google":
		var payload struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal(contents, &payload); err != nil {
			return nil, errors.New("AI provider returned invalid JSON")
		}
		if len(payload.Candidates) > 0 && len(payload.Candidates[0].Content.Parts) > 0 {
			return []byte(payload.Candidates[0].Content.Parts[0].Text), nil
		}
	default:
		var payload struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(contents, &payload); err != nil {
			return nil, errors.New("AI provider returned invalid JSON")
		}
		if len(payload.Choices) > 0 {
			return []byte(payload.Choices[0].Message.Content), nil
		}
	}
	return nil, errors.New("AI provider returned no content")
}
