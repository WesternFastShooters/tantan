package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	DefaultPromptVersion = "prompt-v1"
	EnrichmentSchemaName = "ai-enrichment-v1"
	TopicSchemaName      = "topic-classification-v1"
	SchemaVersion        = "schema-v1"
)

type Preset struct {
	ID      string
	BaseURL string
	Kind    string
}

var providerPresets = map[string]Preset{
	"openai":     {ID: "openai", BaseURL: "https://api.openai.com/v1", Kind: "openai"},
	"anthropic":  {ID: "anthropic", BaseURL: "https://api.anthropic.com", Kind: "anthropic"},
	"google":     {ID: "google", BaseURL: "https://generativelanguage.googleapis.com", Kind: "google"},
	"deepseek":   {ID: "deepseek", BaseURL: "https://api.deepseek.com", Kind: "openai"},
	"openrouter": {ID: "openrouter", BaseURL: "https://openrouter.ai/api/v1", Kind: "openai"},
}

func ProviderPreset(providerID string) (Preset, error) {
	preset, ok := providerPresets[providerID]
	if !ok {
		return Preset{}, errors.New("unsupported AI provider")
	}
	return preset, nil
}

func ValidateModel(model string) error {
	model = strings.TrimSpace(model)
	if count := utf8.RuneCountInString(model); count < 1 || count > 100 || strings.ContainsAny(model, "\r\n\x00") {
		return errors.New("AI model must contain 1 to 100 safe characters")
	}
	return nil
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
