package secrets_test

import (
	"context"
	"strings"
	"testing"

	"tantan.local/tantan-api/internal/secrets"
	"tantan.local/tantan-api/internal/storage"
)

func TestEncryptedStoreRoundTripsWithoutSQLitePlaintext(t *testing.T) {
	ctx := context.Background()
	database, err := storage.Open(ctx, storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	key := []byte("0123456789abcdef0123456789abcdef")
	store, err := secrets.NewStore(secrets.Config{Store: database, Key: key})
	if err != nil {
		t.Fatal(err)
	}
	const account = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const canary = "folo-session-CANARY-never-plaintext"
	if err := store.Set(ctx, account, canary); err != nil {
		t.Fatal(err)
	}
	var nonce, ciphertext []byte
	if err := database.DB().QueryRowContext(ctx, "SELECT nonce,ciphertext FROM secret_records WHERE secret_ref=?", account).Scan(&nonce, &ciphertext); err != nil {
		t.Fatal(err)
	}
	if len(nonce) != 12 || len(ciphertext) <= len(canary) || strings.Contains(string(ciphertext), canary) {
		t.Fatalf("unsafe encrypted record nonce=%d ciphertext=%d", len(nonce), len(ciphertext))
	}
	value, err := store.Get(ctx, account)
	if err != nil || value != canary {
		t.Fatalf("value=%q err=%v", value, err)
	}
	if err := store.Delete(ctx, account); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, account); !secrets.IsNotFound(err) {
		t.Fatalf("deleted secret err=%v", err)
	}
}

func TestEncryptedStoreRejectsWrongKeyAndUnsafeInputsWithoutLeakingSecret(t *testing.T) {
	ctx := context.Background()
	database, err := storage.Open(ctx, storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store, err := secrets.NewStore(secrets.Config{Store: database, Key: []byte("0123456789abcdef0123456789abcdef")})
	if err != nil {
		t.Fatal(err)
	}
	const account = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const canary = "folo-session-CANARY-wrong-key"
	if err := store.Set(ctx, account, canary); err != nil {
		t.Fatal(err)
	}
	wrong, err := secrets.NewStore(secrets.Config{Store: database, Key: []byte("abcdef0123456789abcdef0123456789")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrong.Get(ctx, account); err == nil || strings.Contains(err.Error(), canary) {
		t.Fatalf("wrong key err=%v", err)
	}
	for _, input := range []struct{ account, value string }{{"short", canary}, {account, ""}, {account, "bad\nsecret"}} {
		if err := store.Set(ctx, input.account, input.value); err == nil || input.value != "" && strings.Contains(err.Error(), input.value) {
			t.Fatalf("unsafe input accepted or leaked: %v", err)
		}
	}
}
