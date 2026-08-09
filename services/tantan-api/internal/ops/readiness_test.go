package ops_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/ops"
	"tantan.local/tantan-api/internal/storage"
)

type probeKeychain struct {
	values map[string]string
	err    error
}

func (keychain *probeKeychain) Get(_ context.Context, account string) (string, error) {
	if keychain.err != nil {
		return "", keychain.err
	}
	value, ok := keychain.values[account]
	if !ok {
		return "", errors.New("not found")
	}
	return value, nil
}

func (keychain *probeKeychain) Set(_ context.Context, account, value string) error {
	if keychain.err != nil {
		return keychain.err
	}
	keychain.values[account] = value
	return nil
}

func (keychain *probeKeychain) Delete(_ context.Context, account string) error {
	if keychain.err != nil {
		return keychain.err
	}
	delete(keychain.values, account)
	return nil
}

func openReadinessStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(context.Background(), storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestReadinessFailsClosedForSQLiteMigrationAndKeychainFaults(t *testing.T) {
	t.Run("ready", func(t *testing.T) {
		store := openReadinessStore(t)
		checker, err := ops.NewReadiness(ops.ReadinessConfig{DB: store.DB(), Keychain: &probeKeychain{values: map[string]string{}}, Timeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		result := checker.Check(context.Background())
		if !result.Ready || result.Checks.SQLite != "ok" || result.Checks.Migrations != "ok" || result.Checks.Keychain != "ok" {
			t.Fatalf("readiness=%#v", result)
		}
	})

	t.Run("sqlite closed", func(t *testing.T) {
		store := openReadinessStore(t)
		checker, err := ops.NewReadiness(ops.ReadinessConfig{DB: store.DB(), Keychain: &probeKeychain{values: map[string]string{}}, Timeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		result := checker.Check(context.Background())
		if result.Ready || result.Checks.SQLite != "error" {
			t.Fatalf("closed SQLite readiness=%#v", result)
		}
	})

	t.Run("migration checksum", func(t *testing.T) {
		store := openReadinessStore(t)
		if _, err := store.DB().Exec("UPDATE schema_migrations SET checksum='tampered' WHERE version=2"); err != nil {
			t.Fatal(err)
		}
		checker, err := ops.NewReadiness(ops.ReadinessConfig{DB: store.DB(), Keychain: &probeKeychain{values: map[string]string{}}, Timeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		result := checker.Check(context.Background())
		if result.Ready || result.Checks.Migrations != "error" {
			t.Fatalf("tampered migration readiness=%#v", result)
		}
	})

	t.Run("keychain", func(t *testing.T) {
		store := openReadinessStore(t)
		checker, err := ops.NewReadiness(ops.ReadinessConfig{DB: store.DB(), Keychain: &probeKeychain{values: map[string]string{}, err: errors.New("keychain-CANARY-secret")}, Timeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		result := checker.Check(context.Background())
		if result.Ready || result.Checks.Keychain != "error" {
			t.Fatalf("keychain readiness=%#v", result)
		}
	})
}
