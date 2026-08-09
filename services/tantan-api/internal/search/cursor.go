package search

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrCursorInvalid  = errors.New("search cursor is invalid")
	ErrCursorMismatch = errors.New("search cursor does not match query")
)

type cursorPayload struct {
	Version     int     `json:"v"`
	QueryHash   string  `json:"q"`
	Score       float64 `json:"s"`
	PublishedAt string  `json:"p"`
	EntryID     string  `json:"i"`
}

func encodeCursor(key []byte, payload cursorPayload) (string, error) {
	contents, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode search cursor: %w", err)
	}
	signature := signCursor(key, contents)
	return base64.RawURLEncoding.EncodeToString(contents) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func decodeCursor(key []byte, raw, queryHash string) (cursorPayload, error) {
	var empty cursorPayload
	if len(raw) < 16 || len(raw) > 2048 {
		return empty, ErrCursorInvalid
	}
	separator := -1
	for index := range len(raw) {
		if raw[index] == '.' {
			if separator != -1 {
				return empty, ErrCursorInvalid
			}
			separator = index
		}
	}
	if separator < 1 || separator == len(raw)-1 {
		return empty, ErrCursorInvalid
	}
	contents, err := base64.RawURLEncoding.DecodeString(raw[:separator])
	if err != nil || base64.RawURLEncoding.EncodeToString(contents) != raw[:separator] {
		return empty, ErrCursorInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(raw[separator+1:])
	if err != nil || base64.RawURLEncoding.EncodeToString(signature) != raw[separator+1:] || !hmac.Equal(signature, signCursor(key, contents)) {
		return empty, ErrCursorInvalid
	}
	var payload cursorPayload
	if err := json.Unmarshal(contents, &payload); err != nil || payload.Version != 1 || payload.EntryID == "" || payload.PublishedAt == "" {
		return empty, ErrCursorInvalid
	}
	if payload.QueryHash != queryHash {
		return empty, ErrCursorMismatch
	}
	return payload, nil
}

func signCursor(key, contents []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(contents)
	return mac.Sum(nil)
}
