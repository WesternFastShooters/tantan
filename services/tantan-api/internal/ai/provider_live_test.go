package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"tantan.local/tantan-api/internal/ai"
	"tantan.local/tantan-api/internal/keyring"
)

type liveKeyStoreStub struct {
	value string
	err   error
}

func (store *liveKeyStoreStub) Get(context.Context, string) (string, error) {
	return store.value, store.err
}

func (*liveKeyStoreStub) Set(context.Context, string, string) error {
	return errors.New("not implemented")
}

func (*liveKeyStoreStub) Delete(context.Context, string) error {
	return errors.New("not implemented")
}

func TestLiveKeyLoaderFallsBackToServerKeychain(t *testing.T) {
	const canary = "rotated-key-canary"
	apiKey, err := loadLiveAPIKey("", &liveKeyStoreStub{value: canary})
	if err != nil {
		t.Fatalf("load live Keychain credential: %v", err)
	}
	if apiKey != canary {
		t.Fatal("live loader did not use the server Keychain credential")
	}
}

// TestLiveGoogleTranslation is opt-in. The credential is read from the same
// private server-side file or local Keychain item used by the Go service and
// is never printed.
func TestLiveGoogleTranslation(t *testing.T) {
	if os.Getenv("TANTAN_LIVE_AI") != "1" {
		t.Skip("set TANTAN_LIVE_AI=1 after configuring a rotated server Gemini credential")
	}
	apiKey, err := loadLiveAPIKey(os.Getenv("TANTAN_GEMINI_API_KEY_FILE"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		apiKey = ""
	})

	client, err := ai.NewProviderClient(ai.ProviderClientConfig{})
	if err != nil {
		t.Fatalf("create Google client: %v", err)
	}

	output, err := client.Generate(context.Background(), apiKey, ai.GenerationRequest{
		SchemaName:   ai.EnrichmentSchemaName,
		SystemPrompt: "Translate the supplied English into Simplified Chinese and return the approved enrichment JSON object only.",
		UserPrompt:   `{"title":"Fox","content":"The quick brown fox jumps over the lazy dog."}`,
	})
	if err != nil {
		t.Fatalf("run live Google translation: %v", err)
	}

	var result struct {
		ContentZh string `json:"contentZh"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode live translation JSON: %v", err)
	}
	translation := strings.TrimSpace(result.ContentZh)
	if translation == "" || translation == "The quick brown fox jumps over the lazy dog." {
		t.Fatal("live translation did not return translated text")
	}
	if !strings.ContainsFunc(translation, func(character rune) bool {
		return unicode.Is(unicode.Han, character)
	}) {
		t.Fatal("live translation did not contain Simplified Chinese text")
	}
}

func loadLiveAPIKey(path string, store keyring.Store) (string, error) {
	if path != "" {
		if !filepath.IsAbs(path) {
			return "", errors.New("TANTAN_GEMINI_API_KEY_FILE must name an absolute private file")
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return "", errors.New("Gemini key file must be a private regular file")
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return "", errors.New("read Gemini key file failed")
		}
		apiKey := strings.TrimSuffix(strings.TrimSuffix(string(contents), "\n"), "\r")
		clear(contents)
		if strings.TrimSpace(apiKey) == "" {
			return "", errors.New("Gemini key file contains an invalid credential")
		}
		return apiKey, nil
	}

	if store == nil {
		var err error
		store, err = keyring.NewAIProviderStore()
		if err != nil {
			return "", errors.New("open local Gemini Keychain store failed")
		}
	}
	apiKey, err := store.Get(context.Background(), ai.FixedProviderID)
	if err != nil || strings.TrimSpace(apiKey) == "" {
		return "", errors.New("rotated Gemini credential is not configured in the local Keychain")
	}
	return apiKey, nil
}
