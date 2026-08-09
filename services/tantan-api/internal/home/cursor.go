package home

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
)

var (
	ErrCursorInvalid       = errors.New("home cursor is invalid")
	ErrCursorMismatch      = errors.New("home cursor does not match request")
	ErrQueueVersionChanged = errors.New("home queue version changed")
)

type cursorPayload struct {
	Version   int    `json:"v"`
	QueryHash string `json:"q"`
	QueueID   string `json:"queue"`
	QueueVer  int    `json:"queueVersion"`
	AfterRank int    `json:"afterRank"`
}

func homeQueryHash(userID, topicID, filterKey, timezone string) string {
	digest := sha256.Sum256([]byte(userID + "\x00" + topicID + "\x00" + filterKey + "\x00" + timezone))
	return hex.EncodeToString(digest[:])
}

func encodeCursor(key []byte, payload cursorPayload) (string, error) {
	contents, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(contents)
	signature := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(contents) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func decodeCursor(key []byte, encoded string) (cursorPayload, error) {
	dot := -1
	for index := len(encoded) - 1; index >= 0; index-- {
		if encoded[index] == '.' {
			dot = index
			break
		}
	}
	if dot <= 0 || dot == len(encoded)-1 || len(encoded) > 2048 {
		return cursorPayload{}, ErrCursorInvalid
	}
	contents, err := base64.RawURLEncoding.DecodeString(encoded[:dot])
	if err != nil {
		return cursorPayload{}, ErrCursorInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(encoded[dot+1:])
	if err != nil {
		return cursorPayload{}, ErrCursorInvalid
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(contents)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return cursorPayload{}, ErrCursorInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var payload cursorPayload
	if err := decoder.Decode(&payload); err != nil || decoder.Decode(&struct{}{}) != io.EOF || payload.Version != 1 || len(payload.QueryHash) != 64 || payload.QueueID == "" || payload.QueueVer < 1 || payload.AfterRank < 1 || payload.AfterRank > 60 {
		return cursorPayload{}, ErrCursorInvalid
	}
	return payload, nil
}
