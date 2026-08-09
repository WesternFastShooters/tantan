package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

const databaseFilename = "tantan.sqlite"

type Config struct {
	DataDir string
}

type Store struct {
	database *sql.DB
	path     string
	writer   sync.Mutex
}

func Open(ctx context.Context, config Config) (*Store, error) {
	if config.DataDir == "" {
		return nil, errors.New("Tantan data directory is required")
	}
	dataDirectory, err := filepath.Abs(config.DataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}
	if err := os.MkdirAll(dataDirectory, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	if err := os.Chmod(dataDirectory, 0o700); err != nil {
		return nil, fmt.Errorf("secure data directory: %w", err)
	}
	databasePath := filepath.Join(dataDirectory, databaseFilename)
	created, err := ensureDatabaseFile(databasePath)
	if err != nil {
		return nil, err
	}

	dsn := (&url.URL{Scheme: "file", Path: databasePath}).String() + "?_busy_timeout=5000&_foreign_keys=on&_journal_mode=wal&_synchronous=normal&_txlock=immediate"
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		cleanupNewDatabase(databasePath, created)
		return nil, fmt.Errorf("open SQLite: %w", err)
	}
	database.SetMaxOpenConns(8)
	database.SetMaxIdleConns(4)
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		cleanupNewDatabase(databasePath, created)
		return nil, fmt.Errorf("connect SQLite: %w", err)
	}
	if err := applyMigrations(ctx, database); err != nil {
		_ = database.Close()
		cleanupNewDatabase(databasePath, created)
		return nil, err
	}
	store := &Store{database: database, path: databasePath}
	if integrity, err := store.Integrity(ctx); err != nil || integrity != "ok" {
		_ = database.Close()
		cleanupNewDatabase(databasePath, created)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("SQLite integrity check returned %q", integrity)
	}
	return store, nil
}

func ensureDatabaseFile(path string) (bool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err == nil {
		if closeErr := file.Close(); closeErr != nil {
			return true, fmt.Errorf("close new database file: %w", closeErr)
		}
		return true, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return false, fmt.Errorf("create database file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return false, fmt.Errorf("secure database file: %w", err)
	}
	return false, nil
}

func cleanupNewDatabase(path string, created bool) {
	if !created {
		return
	}
	_ = os.Remove(path)
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
}

func (store *Store) DB() *sql.DB {
	return store.database
}

func (store *Store) Path() string {
	return store.path
}

func (store *Store) Close() error {
	return store.database.Close()
}

func (store *Store) Write(ctx context.Context, operation func(*sql.Tx) error) error {
	if operation == nil {
		return errors.New("storage write operation is required")
	}
	store.writer.Lock()
	defer store.writer.Unlock()
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin SQLite write: %w", err)
	}
	defer transaction.Rollback()
	if err := operation(transaction); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit SQLite write: %w", err)
	}
	return nil
}

func (store *Store) Integrity(ctx context.Context) (string, error) {
	var result string
	if err := store.database.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return "", fmt.Errorf("check SQLite integrity: %w", err)
	}
	return result, nil
}
