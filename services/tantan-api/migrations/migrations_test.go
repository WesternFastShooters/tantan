package migrations_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "modernc.org/sqlite"
)

var migrationFiles = []string{
	"0001_core.sql",
	"0002_search_fts.sql",
	"0003_seed_core_topics.sql",
	"0004_mobile_web_v2.sql",
}

func TestApprovedMigrationsApplyExactlyOnce(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	migrationsDir := filepath.Dir(filename)
	repositoryRoot := filepath.Clean(filepath.Join(migrationsDir, "..", "..", ".."))

	databasePath := filepath.Join(t.TempDir(), "contract.sqlite")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	database.SetMaxOpenConns(1)

	context := context.Background()
	for pass := 1; pass <= 2; pass++ {
		for version, name := range migrationFiles {
			approved, err := os.ReadFile(filepath.Join(repositoryRoot, "spec-package", "db", name))
			if err != nil {
				t.Fatalf("read approved migration %s: %v", name, err)
			}
			generated, err := os.ReadFile(filepath.Join(migrationsDir, name))
			if err != nil {
				t.Fatalf("read generated migration %s: %v", name, err)
			}
			if string(generated) != string(approved) {
				t.Fatalf("generated migration %s differs byte-for-byte", name)
			}

			applied := false
			if _, err := database.ExecContext(context, "SELECT 1 FROM schema_migrations LIMIT 1"); err == nil {
				var found int
				err := database.QueryRowContext(context, "SELECT 1 FROM schema_migrations WHERE version = ?", version+1).Scan(&found)
				if err != nil && err != sql.ErrNoRows {
					t.Fatalf("query migration %d: %v", version+1, err)
				}
				applied = err == nil
			}
			if applied {
				continue
			}
			if _, err := database.ExecContext(context, string(generated)); err != nil {
				t.Fatalf("apply migration %s on pass %d: %v", name, pass, err)
			}
		}
	}

	var migrationCount int
	if err := database.QueryRowContext(context, "SELECT COUNT(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != 4 {
		t.Fatalf("expected 4 migrations exactly once, got %d", migrationCount)
	}

	foreignKeyRows, err := database.QueryContext(context, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatalf("foreign key check: %v", err)
	}
	defer foreignKeyRows.Close()
	if foreignKeyRows.Next() {
		t.Fatal("foreign_key_check returned a violation")
	}

	var integrity string
	if err := database.QueryRowContext(context, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		t.Fatalf("integrity check: %v", err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check = %q, want ok", integrity)
	}
}
