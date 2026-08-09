package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"tantan.local/tantan-api/internal/keyring"
)

const (
	fixedGeminiProviderID = "google-gemini-openai"
	fixedGeminiModel      = "gemini-3.5-flash-lite"
	fixedGeminiEndpoint   = "https://generativelanguage.googleapis.com/v1beta/openai"
	maximumSecretBytes    = 8 * 1024
)

type serverAISecretStore struct {
	value []byte
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
