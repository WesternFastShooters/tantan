package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	version  int
	name     string
	checksum string
}

var approvedMigrations = []migration{
	{version: 1, name: "0001_core.sql", checksum: "spec-package-1.0.0-0001-core"},
	{version: 2, name: "0002_search_fts.sql", checksum: "spec-package-1.0.0-0002-search-fts"},
	{version: 3, name: "0003_seed_core_topics.sql", checksum: "spec-package-1.0.0-0003-core-topic-templates"},
}

func applyMigrations(ctx context.Context, database *sql.DB) error {
	for _, item := range approvedMigrations {
		applied, err := migrationStatus(ctx, database, item)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		contents, err := migrationFiles.ReadFile("migrations/" + item.name)
		if err != nil {
			return fmt.Errorf("read embedded migration %s: %w", item.name, err)
		}
		if _, err := database.ExecContext(ctx, string(contents)); err != nil {
			return fmt.Errorf("apply migration %s: %w", item.name, err)
		}
	}
	var unexpected int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version > ?", len(approvedMigrations)).Scan(&unexpected); err != nil {
		return fmt.Errorf("check schema version: %w", err)
	}
	if unexpected != 0 {
		return errors.New("database schema is newer than this Tantan build")
	}
	return nil
}

func migrationStatus(ctx context.Context, database *sql.DB, item migration) (bool, error) {
	var tableExists int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'").Scan(&tableExists); err != nil {
		return false, fmt.Errorf("inspect migration table: %w", err)
	}
	if tableExists == 0 {
		return false, nil
	}
	var checksum string
	err := database.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version = ?", item.version).Scan(&checksum)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read migration %d: %w", item.version, err)
	}
	if checksum != item.checksum {
		return false, fmt.Errorf("migration %d checksum mismatch", item.version)
	}
	return true, nil
}
