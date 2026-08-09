package main

import (
	"context"
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
