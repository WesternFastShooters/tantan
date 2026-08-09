package session

import (
	"context"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const FoloSessionService = "tantan.folo.session"

var ErrSecretNotFound = errors.New("secret not found")

type SecretStore interface {
	Get(ctx context.Context, account string) (string, error)
	Set(ctx context.Context, account, value string) error
	Delete(ctx context.Context, account string) error
}

type KeyringSecretStore struct {
	service string
}

func NewKeyringSecretStore(service string) (*KeyringSecretStore, error) {
	if service == "" {
		return nil, errors.New("keyring service is required")
	}
	return &KeyringSecretStore{service: service}, nil
}

func (store *KeyringSecretStore) Get(_ context.Context, account string) (string, error) {
	value, err := keyring.Get(store.service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrSecretNotFound
	}
	if err != nil {
		return "", fmt.Errorf("keyring get: %w", err)
	}
	return value, nil
}

func (store *KeyringSecretStore) Set(_ context.Context, account, value string) error {
	if account == "" || value == "" {
		return errors.New("keyring account and value are required")
	}
	if err := keyring.Set(store.service, account, value); err != nil {
		return fmt.Errorf("keyring set: %w", err)
	}
	return nil
}

func (store *KeyringSecretStore) Delete(_ context.Context, account string) error {
	err := keyring.Delete(store.service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("keyring delete: %w", err)
	}
	return nil
}
