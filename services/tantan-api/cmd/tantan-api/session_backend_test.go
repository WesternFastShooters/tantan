package main

import (
	"context"
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
	created, err := sessions.Create(ctx, raw, session.User{ID: "user_1", Name: "Persistent User"}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := sessions.LookupRaw(ctx, raw)
	if err != nil || !ok || loaded.IDHash != created.IDHash || loaded.User.Name != "Persistent User" {
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
