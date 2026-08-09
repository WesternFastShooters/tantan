package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/session"
	"tantan.local/tantan-api/internal/storage"
)

func TestSQLiteSessionBackendPersistsOnlyHashAndAccountProfile(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	backend, err := newSQLiteSessionBackend(store)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := session.NewStoreWithBackend(func() time.Time { return now }, backend)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := session.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	csrf, err := session.NewCSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	email := "person@example.invalid"
	created, err := sessions.CreateWithCSRF(ctx, raw, csrf, session.User{ID: "user_1", Name: "Persistent User", Email: &email}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := sessions.LookupRaw(ctx, raw)
	if err != nil || !ok || loaded.IDHash != created.IDHash || loaded.User.Name != "Persistent User" || loaded.User.Email == nil || *loaded.User.Email != email || !session.ValidCSRF(loaded, csrf) {
		t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err)
	}
	var storedHash, accountName string
	if err := store.DB().QueryRowContext(ctx, "SELECT id_hash FROM local_sessions").Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT name FROM accounts WHERE user_id='user_1'").Scan(&accountName); err != nil {
		t.Fatal(err)
	}
	if storedHash != session.HashToken(raw) || strings.Contains(storedHash, raw) || accountName != "Persistent User" {
		t.Fatalf("hash=%q account=%q", storedHash, accountName)
	}
}

func TestSQLiteTokenReplayStoreReservesOnceAndCanRelease(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, storage.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	replays, err := newSQLiteTokenReplayStore(store)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("one-time-token-test"))
	tokenHash := hex.EncodeToString(digest[:])
	for attempt, want := range []bool{true, false} {
		reserved, reserveErr := replays.Reserve(ctx, tokenHash, time.Now().UTC().Add(time.Hour))
		if reserveErr != nil || reserved != want {
			t.Fatalf("attempt=%d reserved=%v err=%v", attempt, reserved, reserveErr)
		}
	}
	if err := replays.Release(ctx, tokenHash); err != nil {
		t.Fatal(err)
	}
	reserved, err := replays.Reserve(ctx, tokenHash, time.Now().UTC().Add(time.Hour))
	if err != nil || !reserved {
		t.Fatalf("reservation after release=%v err=%v", reserved, err)
	}
}
