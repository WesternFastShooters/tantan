package keyring

import (
	"context"
	"errors"
	"fmt"
	"strings"

	oskeyring "github.com/zalando/go-keyring"
)

const AIProviderService = "tantan.ai.provider"

var ErrNotFound = errors.New("keyring secret not found")

type Store interface {
	Get(ctx context.Context, account string) (string, error)
	Set(ctx context.Context, account, value string) error
	Delete(ctx context.Context, account string) error
}

type OSStore struct {
	service string
}

func NewOSStore(service string) (*OSStore, error) {
	if strings.TrimSpace(service) == "" {
		return nil, errors.New("keyring service is required")
	}
	return &OSStore{service: service}, nil
}

func NewAIProviderStore() (*OSStore, error) {
	return NewOSStore(AIProviderService)
}

func (store *OSStore) Get(_ context.Context, account string) (string, error) {
	if strings.TrimSpace(account) == "" {
		return "", errors.New("keyring account is required")
	}
	value, err := oskeyring.Get(store.service, account)
	if errors.Is(err, oskeyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("keyring get failed")
	}
	return value, nil
}

func (store *OSStore) Set(_ context.Context, account, value string) error {
	if strings.TrimSpace(account) == "" || value == "" {
		return errors.New("keyring account and value are required")
	}
	if err := oskeyring.Set(store.service, account, value); err != nil {
		return errors.New("keyring set failed")
	}
	return nil
}

func (store *OSStore) Delete(_ context.Context, account string) error {
	if strings.TrimSpace(account) == "" {
		return errors.New("keyring account is required")
	}
	err := oskeyring.Delete(store.service, account)
	if errors.Is(err, oskeyring.ErrNotFound) {
		return nil
	}
	if err != nil {
		return errors.New("keyring delete failed")
	}
	return nil
}
