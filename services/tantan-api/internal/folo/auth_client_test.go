package folo_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"tantan.local/tantan-api/internal/folo"
)

func TestAuthClientExchangesOneTimeTokenUsingControlledCookie(t *testing.T) {
	const oneTimeToken = "one-time-token-CANARY-123456"
	const upstreamToken = "upstream-session-CANARY-123456"
	var applyCalls int
	var sessionCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/better-auth/one-time-token/apply":
			applyCalls++
			if request.Method != http.MethodPost || request.Header.Get("Cookie") != "" {
				t.Fatalf("unsafe apply request method=%s cookie=%q", request.Method, request.Header.Get("Cookie"))
			}
			var payload map[string]string
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil || payload["token"] != oneTimeToken {
				t.Fatalf("apply payload=%v err=%v", payload, err)
			}
			http.SetCookie(writer, &http.Cookie{Name: "__Secure-better-auth.session_token", Value: upstreamToken, Path: "/", HttpOnly: true})
			_, _ = io.WriteString(writer, `{"ok":true}`)
		case "/better-auth/get-session":
			sessionCalls++
			if request.Method != http.MethodGet || request.Header.Get("Cookie") != "__Secure-better-auth.session_token="+upstreamToken {
				t.Fatalf("session request method=%s cookie=%q", request.Method, request.Header.Get("Cookie"))
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"user":{"id":"user_1","name":"Test User","email":null,"image":null},"session":{"token":"must-not-be-used"}}`)
		default:
			t.Fatalf("unexpected upstream path %s", request.URL.Path)
		}
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	client, err := folo.NewAuthClient(upstreamURL, upstream.Client())
	if err != nil {
		t.Fatalf("create auth client: %v", err)
	}

	token, user, err := client.ApplyOneTimeToken(context.Background(), oneTimeToken)
	if err != nil {
		t.Fatalf("apply one-time token: %v", err)
	}
	if token != upstreamToken || user.ID != "user_1" || user.Name != "Test User" {
		t.Fatalf("result token=%q user=%#v", token, user)
	}
	if applyCalls != 1 || sessionCalls != 1 {
		t.Fatalf("applyCalls=%d sessionCalls=%d", applyCalls, sessionCalls)
	}
}
