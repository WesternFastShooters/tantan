package ai_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"tantan.local/tantan-api/internal/ai"
)

// TestLiveGoogleTranslation is opt-in. The credential is read from the same
// private server-side file used by the Go service and is never printed.
func TestLiveGoogleTranslation(t *testing.T) {
	if os.Getenv("TANTAN_LIVE_AI") != "1" {
		t.Skip("set TANTAN_LIVE_AI=1 after configuring a rotated server Gemini key file")
	}
	path := os.Getenv("TANTAN_GEMINI_API_KEY_FILE")
	if path == "" || !filepath.IsAbs(path) {
		t.Fatal("TANTAN_GEMINI_API_KEY_FILE must name an absolute private file")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		t.Fatal("Gemini key file must be a private regular file")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read Gemini key file failed")
	}
	apiKey := strings.TrimSuffix(strings.TrimSuffix(string(contents), "\n"), "\r")
	clear(contents)
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
