package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"tantan.local/tantan-api/internal/keyring"
	"tantan.local/tantan-api/internal/storage"
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
	Store   *storage.Store
	Secrets SecretStore
	Now     func() time.Time
}

type SettingsService struct {
	store   *storage.Store
	secrets SecretStore
	now     func() time.Time
}

type ProviderInput struct {
	ProviderID string
	Model      string
	APIKey     string
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
	result := response{Configured: settings.Configured, HasAPIKey: settings.HasAPIKey}
	if settings.ProviderID != "" {
		result.ProviderID = &settings.ProviderID
	}
	if settings.Model != "" {
		result.Model = &settings.Model
	}
	if settings.BaseURL != "" {
		result.BaseURL = &settings.BaseURL
	}
	if settings.KeyFingerprint != "" {
		result.KeyFingerprint = &settings.KeyFingerprint
	}
	return json.Marshal(result)
}

func NewSettingsService(config SettingsConfig) (*SettingsService, error) {
	if config.Store == nil || config.Secrets == nil {
		return nil, errors.New("AI settings storage and Keychain are required")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &SettingsService{store: config.Store, secrets: config.Secrets, now: now}, nil
}

func (service *SettingsService) Put(ctx context.Context, input ProviderInput) (ProviderSettings, error) {
	preset, err := ProviderPreset(strings.TrimSpace(input.ProviderID))
	if err != nil {
		return ProviderSettings{}, err
	}
	model := strings.TrimSpace(input.Model)
	if err := ValidateModel(model); err != nil {
		return ProviderSettings{}, err
	}
	if input.APIKey != "" {
		if count := utf8.RuneCountInString(input.APIKey); count < 8 || count > 4096 || strings.ContainsAny(input.APIKey, "\r\n\x00") {
			return ProviderSettings{}, errors.New("AI API key must contain 8 to 4096 safe characters")
		}
	}
	fingerprint, err := ProviderFingerprint(preset.ID, model, DefaultPromptVersion, SchemaVersion)
	if err != nil {
		return ProviderSettings{}, err
	}
	previousKey, previousExists, err := service.readSecret(ctx, preset.ID)
	if err != nil {
		return ProviderSettings{}, err
	}
	if input.APIKey == "" && !previousExists {
		return ProviderSettings{}, ErrNotConfigured
	}
	keyChanged := input.APIKey != ""
	if keyChanged {
		if err := service.secrets.Set(ctx, preset.ID, input.APIKey); err != nil {
			return ProviderSettings{}, errors.New("save AI key to Keychain failed")
		}
	}
	now := service.now().UTC().Format(time.RFC3339Nano)
	err = service.store.Write(ctx, func(transaction *sql.Tx) error {
		if _, err := transaction.ExecContext(ctx, "UPDATE ai_provider_configs SET enabled=0 WHERE enabled=1"); err != nil {
			return err
		}
		_, err := transaction.ExecContext(ctx, `
INSERT INTO ai_provider_configs(provider_id,base_url,model,fingerprint,enabled,updated_at)
VALUES(?,?,?,?,1,?)
ON CONFLICT(provider_id) DO UPDATE SET
  base_url=excluded.base_url,
  model=excluded.model,
  fingerprint=excluded.fingerprint,
  enabled=1,
  updated_at=excluded.updated_at`, preset.ID, preset.BaseURL, model, fingerprint, now)
		return err
	})
	if err != nil {
		if keyChanged {
			service.restoreSecret(ctx, preset.ID, previousKey, previousExists)
		}
		return ProviderSettings{}, errors.New("save AI provider settings failed")
	}
	return ProviderSettings{Configured: true, ProviderID: preset.ID, Model: model, BaseURL: preset.BaseURL, HasAPIKey: true, KeyFingerprint: fingerprint}, nil
}

func (service *SettingsService) Get(ctx context.Context) (ProviderSettings, error) {
	active, exists, err := service.active(ctx)
	if err != nil {
		return ProviderSettings{}, err
	}
	if !exists {
		return ProviderSettings{}, nil
	}
	_, hasKey, err := service.readSecret(ctx, active.ProviderID)
	if err != nil {
		return ProviderSettings{}, err
	}
	return ProviderSettings{
		Configured:     true,
		ProviderID:     active.ProviderID,
		Model:          active.Model,
		BaseURL:        active.BaseURL,
		HasAPIKey:      hasKey,
		KeyFingerprint: active.Fingerprint,
	}, nil
}

func (service *SettingsService) Credential(ctx context.Context, promptVersion string) (ActiveProvider, string, error) {
	active, exists, err := service.active(ctx)
	if err != nil {
		return ActiveProvider{}, "", err
	}
	if !exists {
		return ActiveProvider{}, "", ErrNotConfigured
	}
	key, hasKey, err := service.readSecret(ctx, active.ProviderID)
	if err != nil {
		return ActiveProvider{}, "", err
	}
	if !hasKey {
		return ActiveProvider{}, "", ErrNotConfigured
	}
	active.Fingerprint, err = ProviderFingerprint(active.ProviderID, active.Model, promptVersion, SchemaVersion)
	if err != nil {
		return ActiveProvider{}, "", err
	}
	return active, key, nil
}

func (service *SettingsService) Delete(ctx context.Context) error {
	active, exists, err := service.active(ctx)
	if err != nil || !exists {
		return err
	}
	previousKey, hasKey, err := service.readSecret(ctx, active.ProviderID)
	if err != nil {
		return err
	}
	if err := service.secrets.Delete(ctx, active.ProviderID); err != nil {
		return errors.New("delete AI key from Keychain failed")
	}
	err = service.store.Write(ctx, func(transaction *sql.Tx) error {
		_, err := transaction.ExecContext(ctx, "DELETE FROM ai_provider_configs WHERE provider_id=? AND enabled=1", active.ProviderID)
		return err
	})
	if err != nil {
		if hasKey {
			_ = service.secrets.Set(ctx, active.ProviderID, previousKey)
		}
		return errors.New("delete AI provider settings failed")
	}
	return nil
}

func (service *SettingsService) active(ctx context.Context) (ActiveProvider, bool, error) {
	var active ActiveProvider
	err := service.store.DB().QueryRowContext(ctx, `
SELECT provider_id,model,base_url,COALESCE(fingerprint,'')
FROM ai_provider_configs WHERE enabled=1`).Scan(&active.ProviderID, &active.Model, &active.BaseURL, &active.Fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return ActiveProvider{}, false, nil
	}
	if err != nil {
		return ActiveProvider{}, false, errors.New("read AI provider settings failed")
	}
	preset, err := ProviderPreset(active.ProviderID)
	if err != nil || preset.BaseURL != active.BaseURL || ValidateModel(active.Model) != nil {
		return ActiveProvider{}, false, errors.New("stored AI provider settings are invalid")
	}
	return active, true, nil
}

func (service *SettingsService) readSecret(ctx context.Context, providerID string) (string, bool, error) {
	value, err := service.secrets.Get(ctx, providerID)
	if errors.Is(err, ErrSecretNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, errors.New("read AI key from Keychain failed")
	}
	if value == "" {
		return "", false, errors.New("Keychain returned an empty AI key")
	}
	return value, true, nil
}

func (service *SettingsService) restoreSecret(ctx context.Context, providerID, previous string, existed bool) {
	if existed {
		_ = service.secrets.Set(ctx, providerID, previous)
		return
	}
	_ = service.secrets.Delete(ctx, providerID)
}
