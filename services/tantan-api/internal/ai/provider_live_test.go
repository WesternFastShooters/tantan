package ai_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"unicode"

	"tantan.local/tantan-api/internal/ai"
	"tantan.local/tantan-api/internal/keyring"
)

const liveGoogleModel = "gemini-3.5-flash-lite"

// TestLiveGoogleTranslation is opt-in because it uses the user's real Provider
// account. The credential is read directly from the OS Keychain and is never
// accepted through an environment variable, fixture, URL, or test output.
func TestLiveGoogleTranslation(t *testing.T) {
	if os.Getenv("TANTAN_LIVE_AI") != "1" {
		t.Skip("set TANTAN_LIVE_AI=1 after saving the Google key in the OS Keychain")
	}

	secrets, err := keyring.NewAIProviderStore()
	if err != nil {
		t.Fatalf("open AI Provider Keychain: %v", err)
	}
	apiKey, err := secrets.Get(context.Background(), "google")
	if err != nil {
		t.Fatalf("read Google key from Keychain: %v", err)
	}
	t.Cleanup(func() {
		apiKey = ""
	})

	client, err := ai.NewProviderClient(ai.ProviderClientConfig{
		ProviderID: "google",
		Model:      liveGoogleModel,
	})
	if err != nil {
		t.Fatalf("create Google client: %v", err)
	}

	output, err := client.Generate(context.Background(), apiKey, ai.GenerationRequest{
		SchemaName:   "live-translation-smoke-v1",
		SystemPrompt: `Translate English into Simplified Chinese. Return only one JSON object matching {"translation":"string"}. Preserve product names.`,
		UserPrompt:   `Translate: "The quick brown fox jumps over the lazy dog."`,
	})
	if err != nil {
		t.Fatalf("run live Google translation: %v", err)
	}

	var result struct {
		Translation string `json:"translation"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode live translation JSON: %v", err)
	}
	translation := strings.TrimSpace(result.Translation)
	if translation == "" || translation == "The quick brown fox jumps over the lazy dog." {
		t.Fatal("live translation did not return translated text")
	}
	if !strings.ContainsFunc(translation, func(character rune) bool {
		return unicode.Is(unicode.Han, character)
	}) {
		t.Fatal("live translation did not contain Simplified Chinese text")
	}
}
