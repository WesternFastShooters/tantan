package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"tantan.local/tantan-api/internal/ai/schema"
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
	Transport http.RoundTripper
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
	preset, err := ProviderPreset(FixedProviderID)
	if err != nil {
		return nil, err
	}
	if err := ValidateModel(FixedModel); err != nil {
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
	return &ProviderClient{preset: preset, model: FixedModel, client: client}, nil
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
	request.Header.Set("Authorization", "Bearer "+apiKey)
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
	responseFormat, err := fixedResponseFormat(generation.SchemaName)
	if err != nil {
		return "", nil, err
	}
	target := strings.TrimSuffix(client.preset.BaseURL, "/") + "/chat/completions"
	payload := map[string]any{
		"model":           client.model,
		"response_format": responseFormat,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", nil, errors.New("encode AI provider request failed")
	}
	return target, body, nil
}

func (client *ProviderClient) response(contents []byte) ([]byte, error) {
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
	return nil, errors.New("AI provider returned no content")
}

func fixedResponseFormat(schemaName string) (map[string]any, error) {
	if schemaName == ConnectionSchemaName {
		return map[string]any{"type": "json_object"}, nil
	}
	approved := map[string]string{
		EnrichmentSchemaName: EnrichmentSchemaName + ".schema.json",
		TopicSchemaName:      TopicSchemaName + ".schema.json",
		FilterSchemaName:     FilterSchemaName + ".schema.json",
	}
	filename, ok := approved[schemaName]
	if !ok {
		return nil, errors.New("AI output schema is not approved")
	}
	contents, err := schema.Read(filename)
	if err != nil {
		return nil, errors.New("read approved AI output schema failed")
	}
	var document map[string]any
	if err := json.Unmarshal(contents, &document); err != nil {
		return nil, errors.New("approved AI output schema is invalid")
	}
	normalizeProviderSchema(document)
	if schemaName == EnrichmentSchemaName {
		requireProviderTranslation(document)
	}
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   schemaName,
			"strict": true,
			"schema": document,
		},
	}, nil
}

// Gemini's OpenAI-compatible structured-output implementation needs an
// explicit primitive type for constant values. Keep the byte-for-byte approved
// schema snapshots untouched and adapt only the schema sent to the fixed
// provider endpoint.
func normalizeProviderSchema(value any) {
	switch typed := value.(type) {
	case map[string]any:
		if constant, ok := typed["const"]; ok {
			delete(typed, "const")
			typed["enum"] = []any{constant}
			if _, present := typed["type"]; !present {
				switch constant.(type) {
				case float64:
					typed["type"] = "integer"
				case string:
					typed["type"] = "string"
				case bool:
					typed["type"] = "boolean"
				}
			}
		}
		for _, child := range typed {
			normalizeProviderSchema(child)
		}
	case []any:
		for _, child := range typed {
			normalizeProviderSchema(child)
		}
	}
}

func requireProviderTranslation(document map[string]any) {
	properties, ok := document["properties"].(map[string]any)
	if !ok {
		return
	}
	for _, field := range []string{"titleZh", "contentZh"} {
		property, ok := properties[field].(map[string]any)
		if !ok {
			continue
		}
		property["type"] = "string"
		property["minLength"] = float64(1)
	}
}
