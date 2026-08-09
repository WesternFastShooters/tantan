package ops

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

const defaultProbeTimeout = 5 * time.Second

var approvedMigrationChecksums = map[int]string{
	1: "spec-package-1.0.0-0001-core",
	2: "spec-package-1.0.0-0002-search-fts",
	3: "spec-package-1.0.0-0003-core-topic-templates",
	4: "spec-package-2.0.0-0004-mobile-web-v2",
}

type Keychain interface {
	Get(ctx context.Context, account string) (string, error)
	Set(ctx context.Context, account, value string) error
	Delete(ctx context.Context, account string) error
}

type ReadinessConfig struct {
	DB       *sql.DB
	Keychain Keychain
	Timeout  time.Duration
}

type Readiness struct {
	database *sql.DB
	keychain Keychain
	timeout  time.Duration
}

type ReadinessChecks struct {
	SQLite     string `json:"sqlite"`
	Migrations string `json:"migrations"`
	Keychain   string `json:"keychain"`
}

type ReadinessResult struct {
	Ready  bool            `json:"ready"`
	Checks ReadinessChecks `json:"checks"`
}

func NewReadiness(config ReadinessConfig) (*Readiness, error) {
	if config.DB == nil || config.Keychain == nil {
		return nil, errors.New("readiness SQLite and Keychain are required")
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultProbeTimeout
	}
	return &Readiness{database: config.DB, keychain: config.Keychain, timeout: config.Timeout}, nil
}

func (readiness *Readiness) Check(ctx context.Context) ReadinessResult {
	result := ReadinessResult{Checks: ReadinessChecks{SQLite: "error", Migrations: "error", Keychain: "error"}}
	if readiness == nil || readiness.database == nil || readiness.keychain == nil {
		return result
	}
	if readiness.checkSQLite(ctx) == nil {
		result.Checks.SQLite = "ok"
	}
	if CheckMigrations(ctx, readiness.database) == nil {
		result.Checks.Migrations = "ok"
	}
	probeContext, cancel := context.WithTimeout(ctx, readiness.timeout)
	defer cancel()
	if ProbeKeychain(probeContext, readiness.keychain) == nil {
		result.Checks.Keychain = "ok"
	}
	result.Ready = result.Checks.SQLite == "ok" && result.Checks.Migrations == "ok" && result.Checks.Keychain == "ok"
	return result
}

func (readiness *Readiness) checkSQLite(ctx context.Context) error {
	checkContext, cancel := context.WithTimeout(ctx, readiness.timeout)
	defer cancel()
	var one int
	if err := readiness.database.QueryRowContext(checkContext, "SELECT 1").Scan(&one); err != nil || one != 1 {
		return errors.New("SQLite query failed")
	}
	var integrity string
	if err := readiness.database.QueryRowContext(checkContext, "PRAGMA quick_check(1)").Scan(&integrity); err != nil || integrity != "ok" {
		return errors.New("SQLite quick check failed")
	}
	return nil
}

func CheckMigrations(ctx context.Context, database *sql.DB) error {
	if database == nil {
		return errors.New("migration database is required")
	}
	var count int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil || count != len(approvedMigrationChecksums) {
		return errors.New("migration count mismatch")
	}
	for version, expected := range approvedMigrationChecksums {
		var checksum string
		if err := database.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version=?", version).Scan(&checksum); err != nil || checksum != expected {
			return errors.New("migration checksum mismatch")
		}
	}
	return nil
}

func ProbeKeychain(ctx context.Context, keychain Keychain) error {
	if keychain == nil {
		return errors.New("Keychain is required")
	}
	accountBytes := make([]byte, 16)
	valueBytes := make([]byte, 32)
	if _, err := rand.Read(accountBytes); err != nil {
		return errors.New("Keychain probe identity failed")
	}
	if _, err := rand.Read(valueBytes); err != nil {
		return errors.New("Keychain probe value failed")
	}
	account := "readiness-" + hex.EncodeToString(accountBytes)
	value := hex.EncodeToString(valueBytes)
	done := make(chan error, 1)
	go func() {
		set := false
		if err := keychain.Set(context.Background(), account, value); err != nil {
			done <- errors.New("Keychain set probe failed")
			return
		}
		set = true
		defer func() {
			if set {
				_ = keychain.Delete(context.Background(), account)
			}
		}()
		stored, err := keychain.Get(context.Background(), account)
		if err != nil || subtle.ConstantTimeCompare([]byte(stored), []byte(value)) != 1 {
			done <- errors.New("Keychain get probe failed")
			return
		}
		if err := keychain.Delete(context.Background(), account); err != nil {
			done <- errors.New("Keychain delete probe failed")
			return
		}
		set = false
		done <- nil
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return errors.New("Keychain probe timed out")
	}
}
