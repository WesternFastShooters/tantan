package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"tantan.local/tantan-api/internal/session"
	"tantan.local/tantan-api/internal/storage"
)

const keyVersion = 1

var ErrNotFound = session.ErrSecretNotFound

type Config struct {
	Store  *storage.Store
	Key    []byte
	Random io.Reader
	Now    func() time.Time
}

type Store struct {
	store  *storage.Store
	aead   cipher.AEAD
	random io.Reader
	now    func() time.Time
}

func NewStore(config Config) (*Store, error) {
	if config.Store == nil || len(config.Key) != 32 {
		return nil, errors.New("SQLite store and a 32-byte master key are required")
	}
	block, err := aes.NewCipher(append([]byte(nil), config.Key...))
	if err != nil {
		return nil, errors.New("create secret cipher failed")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("create secret sealer failed")
	}
	random := config.Random
	if random == nil {
		random = rand.Reader
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Store{store: config.Store, aead: aead, random: random, now: now}, nil
}

func (store *Store) Get(ctx context.Context, account string) (string, error) {
	if store == nil || !validAccount(account) {
		return "", errors.New("valid sealed secret account is required")
	}
	var version int
	var nonce, ciphertext []byte
	err := store.store.DB().QueryRowContext(ctx, `
SELECT key_version,nonce,ciphertext
FROM secret_records
WHERE secret_ref=? AND secret_kind='folo_session'`, account).Scan(&version, &nonce, &ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", errors.New("read sealed secret failed")
	}
	if version != keyVersion || len(nonce) != store.aead.NonceSize() || len(ciphertext) < store.aead.Overhead() {
		return "", errors.New("sealed secret metadata is invalid")
	}
	plaintext, err := store.aead.Open(nil, nonce, ciphertext, additionalData(account, version))
	if err != nil {
		return "", errors.New("unseal secret failed")
	}
	value := string(plaintext)
	for index := range plaintext {
		plaintext[index] = 0
	}
	if !validValue(value) {
		return "", errors.New("unsealed secret is invalid")
	}
	return value, nil
}

func (store *Store) Set(ctx context.Context, account, value string) error {
	if store == nil || !validAccount(account) || !validValue(value) {
		return errors.New("valid sealed secret account and value are required")
	}
	nonce := make([]byte, store.aead.NonceSize())
	if _, err := io.ReadFull(store.random, nonce); err != nil {
		return errors.New("generate sealed secret nonce failed")
	}
	plaintext := []byte(value)
	ciphertext := store.aead.Seal(nil, nonce, plaintext, additionalData(account, keyVersion))
	for index := range plaintext {
		plaintext[index] = 0
	}
	now := store.now().UTC().Format(time.RFC3339Nano)
	return store.store.Write(ctx, func(transaction *sql.Tx) error {
		_, err := transaction.ExecContext(ctx, `
INSERT INTO secret_records(secret_ref,owner_id,secret_kind,key_version,nonce,ciphertext,created_at,updated_at)
VALUES(?,?,'folo_session',?,?,?,?,?)
ON CONFLICT(secret_ref) DO UPDATE SET
  owner_id=excluded.owner_id,
  secret_kind=excluded.secret_kind,
  key_version=excluded.key_version,
  nonce=excluded.nonce,
  ciphertext=excluded.ciphertext,
  updated_at=excluded.updated_at`, account, account, keyVersion, nonce, ciphertext, now, now)
		if err != nil {
			return errors.New("save sealed secret failed")
		}
		return nil
	})
}

func (store *Store) Delete(ctx context.Context, account string) error {
	if store == nil || !validAccount(account) {
		return errors.New("valid sealed secret account is required")
	}
	return store.store.Write(ctx, func(transaction *sql.Tx) error {
		if _, err := transaction.ExecContext(ctx, "DELETE FROM secret_records WHERE secret_ref=? AND secret_kind='folo_session'", account); err != nil {
			return errors.New("delete sealed secret failed")
		}
		return nil
	})
}

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

func additionalData(account string, version int) []byte {
	return []byte("tantan|folo_session|" + account + "|v" + strconv.Itoa(version))
}

func validAccount(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validValue(value string) bool {
	return len(value) >= 1 && len(value) <= 4096 && !strings.ContainsAny(value, "\r\n\x00")
}
