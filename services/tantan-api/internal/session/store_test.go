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

func TestLocalSessionStoresOnlyCSRFHashAndValidatesInConstantTime(t *testing.T) {
	now := time.Date(2026, 8, 10, 2, 0, 0, 0, time.UTC)
	store := session.NewStore(func() time.Time { return now })
	raw, err := session.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	csrf, err := session.NewCSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.CreateWithCSRF(context.Background(), raw, csrf, session.User{ID: "user_csrf", Name: "CSRF User"}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if record.CSRFHash != session.HashCSRF(csrf) || record.SecretRef != record.IDHash {
		t.Fatalf("record did not persist v2 session metadata: %#v", record)
	}
	if strings.Contains(record.CSRFHash, csrf) || !session.ValidCSRF(record, csrf) || session.ValidCSRF(record, csrf+"wrong") {
		t.Fatal("CSRF validation accepted the wrong value or retained plaintext")
	}
}
