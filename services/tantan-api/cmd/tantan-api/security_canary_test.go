package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tantan.local/tantan-api/internal/ai"
	"tantan.local/tantan-api/internal/ops"
	"tantan.local/tantan-api/internal/storage"
)

func TestReleaseSecretCanaryStaysOutsideResponsesSQLiteLogsAndBackup(t *testing.T) {
	canaryPath, canary := releaseSecurityCanary(t)
	key, err := loadGeminiAPIKeyFile(canaryPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { clear(key) })
	secrets, err := newServerAISecretStore(key)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := ai.NewSettingsService(ai.SettingsConfig{Secrets: secrets})
	if err != nil {
		t.Fatal(err)
	}
	publicSettings, err := settings.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	response, err := json.Marshal(publicSettings)
	if err != nil {
		t.Fatal(err)
	}
	assertCanaryAbsent(t, "HTTP response", response, canary)

	var logOutput bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logOutput, nil))
	logger.Info("ai_provider_status", slog.String("providerId", publicSettings.ProviderID), slog.String("model", publicSettings.Model), slog.Bool("configured", publicSettings.Configured))
	assertCanaryAbsent(t, "log", logOutput.Bytes(), canary)

	ctx := context.Background()
	dataDirectory := t.TempDir()
	store, err := storage.Open(ctx, storage.Config{DataDir: dataDirectory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	assertCanaryAbsentFile(t, "SQLite", store.Path(), canary)
	backupPath := filepath.Join(t.TempDir(), "security-backup.sqlite")
	backup, err := ops.Backup(ctx, store, backupPath)
	if err != nil || backup.Integrity != "ok" {
		t.Fatalf("backup integrity=%q err=%v", backup.Integrity, err)
	}
	assertCanaryAbsentFile(t, "backup", backupPath, canary)
}

func releaseSecurityCanary(t *testing.T) (string, string) {
	t.Helper()
	if configured := os.Getenv("TANTAN_SECURITY_CANARY_FILE"); configured != "" {
		contents, err := os.ReadFile(configured)
		if err != nil {
			t.Fatal("read external security canary failed")
		}
		return configured, strings.TrimSpace(string(contents))
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		t.Fatal("generate security canary failed")
	}
	canary := "AQ.TANTAN_RELEASE_" + hex.EncodeToString(random)
	path := filepath.Join(t.TempDir(), "gemini.key")
	if err := os.WriteFile(path, []byte(canary), 0o600); err != nil {
		t.Fatal("write security canary failed")
	}
	return path, canary
}

func assertCanaryAbsent(t *testing.T, location string, contents []byte, canary string) {
	t.Helper()
	if canary == "" || bytes.Contains(contents, []byte(canary)) {
		t.Fatalf("secret canary leaked into %s", location)
	}
}

func assertCanaryAbsentFile(t *testing.T, location, path, canary string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s failed: %v", location, err)
	}
	assertCanaryAbsent(t, location, contents, canary)
}
