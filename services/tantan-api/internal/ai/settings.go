package ai

import (
	"context"
	"encoding/json"
	"errors"

	"tantan.local/tantan-api/internal/keyring"
)

var (
	ErrSecretNotFound = keyring.ErrNotFound
	ErrNotConfigured  = errors.New("AI provider is not configured")
)

type SecretStore interface {
	Get(ctx context.Context, account string) (string, error)
	Set(ctx context.Context, account, value string) error
	Delete(ctx context.Context, account string) error
}

type SettingsConfig struct {
	Secrets SecretStore
}

type SettingsService struct {
	secrets SecretStore
}

type ProviderSettings struct {
	Configured     bool
	ProviderID     string
	Model          string
	BaseURL        string
	HasAPIKey      bool
	KeyFingerprint string
}

type ActiveProvider struct {
	ProviderID  string
	Model       string
	BaseURL     string
	Fingerprint string
}

func (settings ProviderSettings) MarshalJSON() ([]byte, error) {
	type response struct {
		Configured     bool    `json:"configured"`
		ProviderID     *string `json:"providerId"`
		Model          *string `json:"model"`
		BaseURL        *string `json:"baseUrl"`
		HasAPIKey      bool    `json:"hasApiKey"`
		KeyFingerprint *string `json:"keyFingerprint"`
	}
	providerID := settings.ProviderID
	model := settings.Model
	baseURL := settings.BaseURL
	result := response{
		Configured: settings.Configured,
		ProviderID: &providerID,
		Model:      &model,
		BaseURL:    &baseURL,
		HasAPIKey:  settings.HasAPIKey,
	}
	if settings.KeyFingerprint != "" {
		result.KeyFingerprint = &settings.KeyFingerprint
	}
	return json.Marshal(result)
}

func NewSettingsService(config SettingsConfig) (*SettingsService, error) {
	if config.Secrets == nil {
		return nil, errors.New("server AI secret configuration is required")
	}
	return &SettingsService{secrets: config.Secrets}, nil
}

func (service *SettingsService) Get(ctx context.Context) (ProviderSettings, error) {
	key, exists, err := service.readSecret(ctx)
	if err != nil {
		return ProviderSettings{}, err
	}
	settings := ProviderSettings{
		Configured: exists,
		ProviderID: FixedProviderID,
		Model:      FixedModel,
		BaseURL:    FixedBaseURL,
		HasAPIKey:  exists,
	}
	if exists {
		settings.KeyFingerprint = KeyFingerprint(key)
	}
	return settings, nil
}

func (service *SettingsService) Credential(ctx context.Context, promptVersion string) (ActiveProvider, string, error) {
	key, exists, err := service.readSecret(ctx)
	if err != nil {
		return ActiveProvider{}, "", err
	}
	if !exists {
		return ActiveProvider{}, "", ErrNotConfigured
	}
	fingerprint, err := ProviderFingerprint(FixedProviderID, FixedModel, promptVersion, SchemaVersion)
	if err != nil {
		return ActiveProvider{}, "", err
	}
	return ActiveProvider{ProviderID: FixedProviderID, Model: FixedModel, BaseURL: FixedBaseURL, Fingerprint: fingerprint}, key, nil
}

func (service *SettingsService) readSecret(ctx context.Context) (string, bool, error) {
	value, err := service.secrets.Get(ctx, FixedProviderID)
	if errors.Is(err, ErrSecretNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, errors.New("read server AI configuration failed")
	}
	if value == "" {
		return "", false, errors.New("server AI configuration is empty")
	}
	return value, true, nil
}
