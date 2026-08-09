package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	FixedProviderID      = "google-gemini-openai"
	FixedModel           = "gemini-3.5-flash-lite"
	FixedBaseURL         = "https://generativelanguage.googleapis.com/v1beta/openai"
	DefaultPromptVersion = "prompt-v1"
	EnrichmentSchemaName = "ai-enrichment-v1"
	TopicSchemaName      = "topic-classification-v1"
	FilterSchemaName     = "filter-spec-v1"
	ConnectionSchemaName = "provider-connection-test-v1"
	SchemaVersion        = "schema-v1"
)

type Preset struct {
	ID      string
	BaseURL string
	Kind    string
}

var providerPresets = map[string]Preset{
	FixedProviderID: {ID: FixedProviderID, BaseURL: FixedBaseURL, Kind: "openai"},
}

func ProviderPreset(providerID string) (Preset, error) {
	preset, ok := providerPresets[providerID]
	if !ok {
		return Preset{}, errors.New("unsupported AI provider")
	}
	return preset, nil
}

func ValidateModel(model string) error {
	if strings.TrimSpace(model) != FixedModel {
		return errors.New("unsupported AI model")
	}
	return nil
}

func KeyFingerprint(apiKey string) string {
	digest := sha256.Sum256([]byte(apiKey))
	return strings.ToUpper(hex.EncodeToString(digest[:4]))
}

func ProviderFingerprint(providerID, model, promptVersion, schemaVersion string) (string, error) {
	preset, err := ProviderPreset(providerID)
	if err != nil {
		return "", err
	}
	if err := ValidateModel(model); err != nil {
		return "", err
	}
	if strings.TrimSpace(promptVersion) == "" || strings.TrimSpace(schemaVersion) == "" {
		return "", errors.New("prompt and schema versions are required")
	}
	digest := sha256.Sum256([]byte(preset.ID + "\x00" + preset.BaseURL + "\x00" + strings.TrimSpace(model) + "\x00" + promptVersion + "\x00" + schemaVersion))
	return hex.EncodeToString(digest[:])[:12], nil
}
