package main

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tantan.local/tantan-api/internal/keyring"
)

func TestServerSecretFilesRequireAbsolutePrivateRegularFiles(t *testing.T) {
	directory := t.TempDir()
	private := filepath.Join(directory, "gemini.key")
	if err := os.WriteFile(private, []byte("gemini-test-key-CANARY\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	value, err := loadGeminiAPIKeyFile(private)
	if err != nil || string(value) != "gemini-test-key-CANARY" {
		t.Fatalf("value length=%d err=%v", len(value), err)
	}
	clear(value)

	worldReadable := filepath.Join(directory, "world.key")
	if err := os.WriteFile(worldReadable, []byte("gemini-test-key-CANARY"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(directory, "link.key")
	if err := os.Symlink(private, symlink); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"relative.key", worldReadable, symlink} {
		if _, err := loadGeminiAPIKeyFile(path); err == nil || strings.Contains(err.Error(), "CANARY") {
			t.Fatalf("unsafe secret path %q err=%v", path, err)
		}
	}
}

func TestServerMasterKeyProvidesContainerCompatibleRuntimeStores(t *testing.T) {
	master := []byte("0123456789abcdef0123456789abcdef")
	first, err := newDerivedCursorSecretStore(master)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newDerivedCursorSecretStore(master)
	if err != nil {
		t.Fatal(err)
	}
	firstValue, err := first.Get(context.Background(), cursorKeyAccount)
	if err != nil {
		t.Fatal(err)
	}
	secondValue, err := second.Get(context.Background(), cursorKeyAccount)
	if err != nil || secondValue != firstValue {
		t.Fatalf("derived cursor secret is not stable err=%v", err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(firstValue)
	if err != nil || len(decoded) != 32 || string(decoded) == string(master) {
		t.Fatalf("invalid derived cursor secret length=%d err=%v", len(decoded), err)
	}
	if _, err := first.Get(context.Background(), "other"); err != keyring.ErrNotFound {
		t.Fatalf("unexpected derived store lookup err=%v", err)
	}

	probe := newEphemeralSecretStore()
	if err := probe.Set(context.Background(), "readiness", "value"); err != nil {
		t.Fatal(err)
	}
	if value, err := probe.Get(context.Background(), "readiness"); err != nil || value != "value" {
		t.Fatalf("probe value=%q err=%v", value, err)
	}
	if err := probe.Delete(context.Background(), "readiness"); err != nil {
		t.Fatal(err)
	}
	if _, err := probe.Get(context.Background(), "readiness"); err != keyring.ErrNotFound {
		t.Fatalf("deleted probe remained available err=%v", err)
	}
}

func TestMasterKeyAndFixedGeminiServerStore(t *testing.T) {
	directory := t.TempDir()
	masterPath := filepath.Join(directory, "master.key")
	if err := os.WriteFile(masterPath, []byte("0123456789abcdef0123456789abcdef"), 0o400); err != nil {
		t.Fatal(err)
	}
	master, err := loadMasterKeyFile(masterPath)
	if err != nil || len(master) != 32 {
		t.Fatalf("master length=%d err=%v", len(master), err)
	}
	clear(master)

	const canary = "gemini-server-only-CANARY"
	store, err := newServerAISecretStore([]byte(canary))
	if err != nil {
		t.Fatal(err)
	}
	value, err := store.Get(context.Background(), fixedGeminiProviderID)
	if err != nil || value != canary {
		t.Fatalf("secret value length=%d err=%v", len(value), err)
	}
	if _, err := store.Get(context.Background(), "openai"); err != keyring.ErrNotFound {
		t.Fatalf("unexpected provider err=%v", err)
	}
	if err := store.Set(context.Background(), fixedGeminiProviderID, "replacement"); err == nil {
		t.Fatal("server AI secret store accepted a browser-style update")
	}
	if err := store.Delete(context.Background(), fixedGeminiProviderID); err == nil {
		t.Fatal("server AI secret store accepted deletion")
	}
	if fixedGeminiModel != "gemini-3.5-flash-lite" || fixedGeminiEndpoint != "https://generativelanguage.googleapis.com/v1beta/openai" {
		t.Fatal("Gemini preset changed")
	}
}

func TestCloudflareEnvironmentSecretsAreValidatedWithoutDisclosure(t *testing.T) {
	master := []byte("0123456789abcdef0123456789abcdef")
	encoded := base64.StdEncoding.EncodeToString(master)
	decoded, err := loadMasterKeyEnvironment(encoded)
	if err != nil || string(decoded) != string(master) {
		t.Fatalf("master length=%d err=%v", len(decoded), err)
	}
	clear(decoded)

	for _, raw := range []string{"", "not-base64", base64.StdEncoding.EncodeToString([]byte("short"))} {
		_, err := loadMasterKeyEnvironment(raw)
		if err == nil || (raw != "" && strings.Contains(err.Error(), raw)) {
			t.Fatalf("unsafe master environment value accepted or disclosed: length=%d err=%v", len(raw), err)
		}
	}

	const geminiCanary = "cloudflare-gemini-CANARY"
	gemini, err := loadGeminiAPIKeyEnvironment(geminiCanary)
	if err != nil || string(gemini) != geminiCanary {
		t.Fatalf("Gemini environment value length=%d err=%v", len(gemini), err)
	}
	clear(gemini)
	if _, err := loadGeminiAPIKeyEnvironment("bad\nCANARY"); err == nil || strings.Contains(err.Error(), "CANARY") {
		t.Fatalf("unsafe Gemini environment value accepted or disclosed: %v", err)
	}
}
