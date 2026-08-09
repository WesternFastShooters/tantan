package session

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

const LocalCookieName = "__Host-tantan_session"

type User struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Email *string `json:"email"`
	Image *string `json:"image"`
}

type Record struct {
	IDHash    string
	SecretRef string
	CSRFHash  string
	User      User
	Timezone  string
	ExpiresAt time.Time
	LastSeen  time.Time
	CreatedAt time.Time
}

type Backend interface {
	SaveSession(ctx context.Context, record Record) error
	FindSession(ctx context.Context, idHash string) (Record, bool, error)
	DeleteSession(ctx context.Context, idHash string) error
}

type Store struct {
	backend Backend
	now     func() time.Time
	memory  *memoryBackend
}

func NewStore(now func() time.Time) *Store {
	memory := newMemoryBackend()
	return newStore(now, memory, memory)
}

func NewStoreWithBackend(now func() time.Time, backend Backend) (*Store, error) {
	if backend == nil {
		return nil, errors.New("session backend is required")
	}
	return newStore(now, backend, nil), nil
}

func newStore(now func() time.Time, backend Backend, memory *memoryBackend) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{backend: backend, now: now, memory: memory}
}

func NewToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func NewCSRFToken() (string, error) {
	return NewToken()
}

func HashToken(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

func HashCSRF(raw string) string {
	return HashToken(raw)
}

func ValidCSRF(record Record, raw string) bool {
	if len(record.CSRFHash) != sha256.Size*2 || len(raw) < 40 {
		return false
	}
	return hmac.Equal([]byte(record.CSRFHash), []byte(HashCSRF(raw)))
}

func (store *Store) Create(ctx context.Context, raw string, user User, expiresAt time.Time) (Record, error) {
	csrf, err := NewCSRFToken()
	if err != nil {
		return Record{}, err
	}
	return store.CreateWithCSRF(ctx, raw, csrf, user, expiresAt)
}

func (store *Store) CreateWithCSRF(ctx context.Context, raw, csrf string, user User, expiresAt time.Time) (Record, error) {
	now := store.now().UTC()
	if len(raw) < 40 || len(csrf) < 40 || strings.TrimSpace(user.ID) == "" || strings.TrimSpace(user.Name) == "" || !expiresAt.After(now) {
		return Record{}, errors.New("invalid local session")
	}
	idHash := HashToken(raw)
	record := Record{
		IDHash:    idHash,
		SecretRef: idHash,
		CSRFHash:  HashCSRF(csrf),
		User:      user,
		Timezone:  "Asia/Shanghai",
		ExpiresAt: expiresAt.UTC(),
		LastSeen:  now,
		CreatedAt: now,
	}
	if err := store.backend.SaveSession(ctx, record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (store *Store) RotateCSRF(ctx context.Context, idHash, csrf string) (Record, error) {
	if len(idHash) != sha256.Size*2 || len(csrf) < 40 {
		return Record{}, errors.New("invalid CSRF rotation")
	}
	record, ok, err := store.backend.FindSession(ctx, idHash)
	if err != nil {
		return Record{}, err
	}
	if !ok {
		return Record{}, errors.New("local session not found")
	}
	record.CSRFHash = HashCSRF(csrf)
	if err := store.backend.SaveSession(ctx, record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (store *Store) LookupRaw(ctx context.Context, raw string) (Record, bool, error) {
	return store.LookupHash(ctx, HashToken(raw))
}

func (store *Store) LookupHash(ctx context.Context, idHash string) (Record, bool, error) {
	now := store.now().UTC()
	record, ok, err := store.backend.FindSession(ctx, idHash)
	if err != nil || !ok {
		return Record{}, ok, err
	}
	if !record.ExpiresAt.After(now) {
		if err := store.backend.DeleteSession(ctx, idHash); err != nil {
			return Record{}, false, err
		}
		return Record{}, false, nil
	}
	if now.Sub(record.LastSeen) >= 5*time.Minute {
		record.LastSeen = now
		if err := store.backend.SaveSession(ctx, record); err != nil {
			return Record{}, false, err
		}
	}
	return record, true, nil
}

func (store *Store) DeleteHash(ctx context.Context, idHash string) error {
	return store.backend.DeleteSession(ctx, idHash)
}

func (store *Store) UpdateTimezone(ctx context.Context, idHash, timezone string) (Record, error) {
	record, ok, err := store.backend.FindSession(ctx, idHash)
	if err != nil {
		return Record{}, err
	}
	if !ok {
		return Record{}, errors.New("local session not found")
	}
	if record.Timezone == timezone {
		return record, nil
	}
	record.Timezone = timezone
	if err := store.backend.SaveSession(ctx, record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (store *Store) Len() int {
	if store.memory == nil {
		return -1
	}
	return store.memory.len()
}

type memoryBackend struct {
	mu       sync.RWMutex
	sessions map[string]Record
}

func newMemoryBackend() *memoryBackend {
	return &memoryBackend{sessions: make(map[string]Record)}
}

func (backend *memoryBackend) SaveSession(_ context.Context, record Record) error {
	backend.mu.Lock()
	backend.sessions[record.IDHash] = record
	backend.mu.Unlock()
	return nil
}

func (backend *memoryBackend) FindSession(_ context.Context, idHash string) (Record, bool, error) {
	backend.mu.RLock()
	record, ok := backend.sessions[idHash]
	backend.mu.RUnlock()
	return record, ok, nil
}

func (backend *memoryBackend) DeleteSession(_ context.Context, idHash string) error {
	backend.mu.Lock()
	delete(backend.sessions, idHash)
	backend.mu.Unlock()
	return nil
}

func (backend *memoryBackend) len() int {
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	return len(backend.sessions)
}

type contextKey struct{}

func WithRecord(ctx context.Context, record Record) context.Context {
	return context.WithValue(ctx, contextKey{}, record)
}

func FromContext(ctx context.Context) (Record, bool) {
	record, ok := ctx.Value(contextKey{}).(Record)
	return record, ok
}
