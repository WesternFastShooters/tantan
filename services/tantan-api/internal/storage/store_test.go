package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/session"
	"tantan.local/tantan-api/internal/storage"
)

func TestOpenAppliesApprovedMigrationsAndSecuresDatabase(t *testing.T) {
	ctx := context.Background()
	dataDirectory := filepath.Join(t.TempDir(), "Tantan")
	store, err := storage.Open(ctx, storage.Config{DataDir: dataDirectory})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	path := store.Path()
	if err := store.Close(); err != nil {
		t.Fatalf("close storage: %v", err)
	}

	directoryInfo, err := os.Stat(dataDirectory)
	if err != nil {
		t.Fatalf("stat data directory: %v", err)
	}
	if directoryInfo.Mode().Perm() != 0o700 {
		t.Fatalf("data directory mode=%#o", directoryInfo.Mode().Perm())
	}
	databaseInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat database: %v", err)
	}
	if databaseInfo.Mode().Perm() != 0o600 {
		t.Fatalf("database mode=%#o", databaseInfo.Mode().Perm())
	}

	store, err = storage.Open(ctx, storage.Config{DataDir: dataDirectory})
	if err != nil {
		t.Fatalf("reopen storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	var migrationCount int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != 4 {
		t.Fatalf("migration count=%d", migrationCount)
	}
	var templateCount int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM core_topic_templates").Scan(&templateCount); err != nil {
		t.Fatalf("count topic templates: %v", err)
	}
	if templateCount != 6 {
		t.Fatalf("core topic template count=%d", templateCount)
	}
	for pragma, expected := range map[string]string{
		"foreign_keys": "1",
		"busy_timeout": "5000",
		"journal_mode": "wal",
	} {
		var actual string
		if err := store.DB().QueryRowContext(ctx, "PRAGMA "+pragma).Scan(&actual); err != nil {
			t.Fatalf("read PRAGMA %s: %v", pragma, err)
		}
		if actual != expected {
			t.Fatalf("PRAGMA %s=%q", pragma, actual)
		}
	}
	if integrity, err := store.Integrity(ctx); err != nil || integrity != "ok" {
		t.Fatalf("integrity=%q err=%v", integrity, err)
	}
}

func TestSQLiteSessionBackendPersistsOnlySessionHash(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	sessions, err := session.NewStoreWithBackend(func() time.Time { return now }, storage.NewSessionBackend(store))
	if err != nil {
		t.Fatalf("create session store: %v", err)
	}
	raw, err := session.NewToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	record, err := sessions.Create(ctx, raw, session.User{ID: "user_1", Name: "Test User"}, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	var storedHash string
	if err := store.DB().QueryRowContext(ctx, "SELECT id_hash FROM local_sessions WHERE user_id = ?", "user_1").Scan(&storedHash); err != nil {
		t.Fatalf("read session: %v", err)
	}
	if storedHash != record.IDHash || storedHash == raw {
		t.Fatalf("stored session id=%q", storedHash)
	}
	var rawMatches int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM local_sessions WHERE id_hash = ?", raw).Scan(&rawMatches); err != nil {
		t.Fatalf("scan raw token: %v", err)
	}
	if rawMatches != 0 {
		t.Fatal("raw local token entered SQLite")
	}
}
