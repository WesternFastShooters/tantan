package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
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
	approvedFiles := map[string]string{
		EnrichmentSchemaName: EnrichmentSchemaName + ".schema.json",
		TopicSchemaName:      TopicSchemaName + ".schema.json",
		FilterSchemaName:     FilterSchemaName + ".schema.json",
	}
	filename, approved := approvedFiles[schemaName]
	if !approved {
		return nil, errors.New("AI output schema is not approved")
	}
	raw, err := schema.Read(filename)
	if err != nil {
		return nil, errors.New("read approved AI output schema failed")
	}
	var approvedSchema map[string]any
	if err := json.Unmarshal(raw, &approvedSchema); err != nil {
		return nil, errors.New("decode approved AI output schema failed")
	}
	compatible, ok := geminiCompatibleSchema(approvedSchema).(map[string]any)
	if !ok {
		return nil, errors.New("convert approved AI output schema failed")
	}
	if schemaName == EnrichmentSchemaName {
		properties, ok := compatible["properties"].(map[string]any)
		if !ok {
			return nil, errors.New("approved enrichment schema has no properties")
		}
		// Only fully translated entries may enter the display pool. Requiring
		// strings here prevents nullable translations from wasting a provider call.
		properties["titleZh"] = map[string]any{"type": "string"}
		properties["contentZh"] = map[string]any{"type": "string"}
	}
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   schemaName,
			"strict": true,
			"schema": compatible,
		},
	}, nil
}

// geminiCompatibleSchema keeps only the JSON Schema subset accepted by
// Gemini structured outputs. Responses still pass the complete approved
// schema locally before being persisted.
func geminiCompatibleSchema(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		converted := make(map[string]any)
		for key, child := range typed {
			switch key {
			case "properties", "$defs":
				children, ok := child.(map[string]any)
				if !ok {
					continue
				}
				convertedChildren := make(map[string]any, len(children))
				for name, childSchema := range children {
					convertedChildren[name] = geminiCompatibleSchema(childSchema)
				}
				converted[key] = convertedChildren
			case "items", "additionalProperties":
				converted[key] = geminiCompatibleSchema(child)
			case "prefixItems", "anyOf", "oneOf":
				children, ok := child.([]any)
				if !ok {
					continue
				}
				convertedChildren := make([]any, 0, len(children))
				for _, childSchema := range children {
					convertedChildren = append(convertedChildren, geminiCompatibleSchema(childSchema))
				}
				converted[key] = convertedChildren
			case "const":
				converted["enum"] = []any{child}
				if _, exists := converted["type"]; !exists {
					if inferred := jsonSchemaType(child); inferred != "" {
						converted["type"] = inferred
					}
				}
			case "type", "format", "enum", "minItems", "maxItems", "minimum", "maximum", "required", "$ref":
				converted[key] = child
			}
		}
		return converted
	case []any:
		converted := make([]any, 0, len(typed))
		for _, child := range typed {
			converted = append(converted, geminiCompatibleSchema(child))
		}
		return converted
	default:
		return typed
	}
}

func jsonSchemaType(value any) string {
	switch typed := value.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		if math.Trunc(typed) == typed {
			return "integer"
		}
		return "number"
	case nil:
		return "null"
	default:
		return ""
	}
}
