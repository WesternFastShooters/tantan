package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"tantan.local/tantan-api/internal/ai"
	"tantan.local/tantan-api/internal/keyring"
)

const (
	fixedGeminiProviderID = ai.FixedProviderID
	fixedGeminiModel      = ai.FixedModel
	fixedGeminiEndpoint   = ai.FixedBaseURL
	maximumSecretBytes    = 8 * 1024
)

type serverAISecretStore struct {
	value []byte
}

type ephemeralSecretStore struct {
	mu     sync.Mutex
	values map[string]string
}

type derivedCursorSecretStore struct {
	value string
}

func newServerAISecretStore(value []byte) (*serverAISecretStore, error) {
	if err := validateGeminiAPIKey(value); err != nil {
		return nil, err
	}
	return &serverAISecretStore{value: append([]byte(nil), value...)}, nil
}

func (store *serverAISecretStore) Get(_ context.Context, account string) (string, error) {
	if store == nil || account != fixedGeminiProviderID || len(store.value) == 0 {
		return "", keyring.ErrNotFound
	}
	return string(store.value), nil
}

func (*serverAISecretStore) Set(context.Context, string, string) error {
	return errors.New("server Gemini configuration is read-only")
}

func (*serverAISecretStore) Delete(context.Context, string) error {
	return errors.New("server Gemini configuration is read-only")
}

func newEphemeralSecretStore() *ephemeralSecretStore {
	return &ephemeralSecretStore{values: make(map[string]string)}
}

func (store *ephemeralSecretStore) Get(_ context.Context, account string) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	value, ok := store.values[account]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (store *ephemeralSecretStore) Set(_ context.Context, account, value string) error {
	if strings.TrimSpace(account) == "" || value == "" {
		return errors.New("secret account and value are required")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.values[account] = value
	return nil
}

func (store *ephemeralSecretStore) Delete(_ context.Context, account string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.values, account)
	return nil
}

func newDerivedCursorSecretStore(masterKey []byte) (*derivedCursorSecretStore, error) {
	if len(masterKey) != 32 {
		return nil, errors.New("cursor key derivation requires a 32-byte master key")
	}
	mac := hmac.New(sha256.New, masterKey)
	_, _ = mac.Write([]byte("tantan/cursor-signing/v1"))
	return &derivedCursorSecretStore{value: base64.RawURLEncoding.EncodeToString(mac.Sum(nil))}, nil
}

func (store *derivedCursorSecretStore) Get(_ context.Context, account string) (string, error) {
	if store == nil || account != cursorKeyAccount {
		return "", keyring.ErrNotFound
	}
	return store.value, nil
}

func (*derivedCursorSecretStore) Set(context.Context, string, string) error {
	return errors.New("derived cursor secret is read-only")
}

func (*derivedCursorSecretStore) Delete(context.Context, string) error {
	return errors.New("derived cursor secret is read-only")
}

func loadMasterKeyFile(path string) ([]byte, error) {
	value, err := readPrivateSecretFile(path)
	if err != nil {
		return nil, err
	}
	if len(value) != 32 {
		clear(value)
		return nil, errors.New("master key file must contain exactly 32 bytes")
	}
	return value, nil
}

func loadMasterKeyEnvironment(raw string) ([]byte, error) {
	value, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(value) != 32 {
		clear(value)
		return nil, errors.New("master key environment variable must contain one base64-encoded 32-byte key")
	}
	return value, nil
}

func loadGeminiAPIKeyFile(path string) ([]byte, error) {
	value, err := readPrivateSecretFile(path)
	if err != nil {
		return nil, err
	}
	if err := validateGeminiAPIKey(value); err != nil {
		clear(value)
		return nil, err
	}
	return value, nil
}

func loadGeminiAPIKeyEnvironment(raw string) ([]byte, error) {
	value := []byte(raw)
	if err := validateGeminiAPIKey(value); err != nil {
		clear(value)
		return nil, errors.New("Gemini API key environment variable contains an invalid credential")
	}
	return value, nil
}

func readPrivateSecretFile(path string) ([]byte, error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, errors.New("secret file path must be absolute")
	}
	linkInfo, err := os.Lstat(path)
	if err != nil || linkInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("secret file is unavailable")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("secret file is unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(linkInfo, info) || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("secret file must be a private regular file")
	}
	contents, err := io.ReadAll(io.LimitReader(file, maximumSecretBytes+1))
	if err != nil || len(contents) == 0 || len(contents) > maximumSecretBytes {
		clear(contents)
		return nil, errors.New("secret file has an invalid size")
	}
	contents = bytes.TrimSuffix(contents, []byte("\n"))
	contents = bytes.TrimSuffix(contents, []byte("\r"))
	return contents, nil
}

func validateGeminiAPIKey(value []byte) error {
	if len(value) < 8 || len(value) > 4096 || !utf8.Valid(value) || strings.ContainsAny(string(value), "\r\n\x00") {
		return errors.New("Gemini API key file contains an invalid credential")
	}
	return nil
}
