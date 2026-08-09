package session_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/session"
)

func TestLocalSessionStoresOnlyHashAndExpires(t *testing.T) {
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	store := session.NewStore(func() time.Time { return now })
	raw, err := session.NewToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if len(raw) < 40 {
		t.Fatalf("local token too short: %d", len(raw))
	}

	record, err := store.Create(context.Background(), raw, session.User{ID: "user_1", Name: "Test User"}, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("create local session: %v", err)
	}
	if record.IDHash != session.HashToken(raw) {
		t.Fatal("record does not use SHA-256 token hash")
	}
	if strings.Contains(record.IDHash, raw) {
		t.Fatal("record contains raw local token")
	}
	if _, ok, err := store.LookupRaw(context.Background(), raw); err != nil || !ok {
		t.Fatal("active local session was not found")
	}

	now = now.Add(2 * time.Hour)
	if _, ok, err := store.LookupRaw(context.Background(), raw); err != nil || ok {
		t.Fatal("expired local session was accepted")
	}
}

func TestLocalSessionRejectsWeakOrAlreadyExpiredInput(t *testing.T) {
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	store := session.NewStore(func() time.Time { return now })
	for _, test := range []struct {
		name      string
		raw       string
		user      session.User
		expiresAt time.Time
	}{
		{name: "weak token", raw: "short", user: session.User{ID: "user_1", Name: "Test"}, expiresAt: now.Add(time.Hour)},
		{name: "missing user", raw: strings.Repeat("a", 40), user: session.User{}, expiresAt: now.Add(time.Hour)},
		{name: "expired", raw: strings.Repeat("a", 40), user: session.User{ID: "user_1", Name: "Test"}, expiresAt: now},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.Create(context.Background(), test.raw, test.user, test.expiresAt); err == nil {
				t.Fatal("invalid session input was accepted")
			}
		})
	}
}
