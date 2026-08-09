package folo_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

func TestAuthClientFallsBackToVerifyThenValidatesSession(t *testing.T) {
	const oneTimeToken = "one-time-token-verify-1234567890"
	const upstreamToken = "verified-session-token-1234567890"
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		switch request.URL.Path {
		case "/better-auth/one-time-token/apply":
			writer.WriteHeader(http.StatusNotFound)
		case "/better-auth/one-time-token/verify":
			http.SetCookie(writer, &http.Cookie{Name: "__Secure-better-auth.session_token", Value: upstreamToken})
			_, _ = io.WriteString(writer, `{"ok":true}`)
		case "/better-auth/get-session":
			if request.Header.Get("Cookie") != "__Secure-better-auth.session_token="+upstreamToken {
				t.Fatalf("session cookie=%q", request.Header.Get("Cookie"))
			}
			_, _ = io.WriteString(writer, `{"user":{"id":"user_verify","name":"Verify User","email":null,"image":null}}`)
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	client, err := folo.NewAuthClient(upstreamURL, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	token, user, err := client.ApplyOneTimeToken(context.Background(), oneTimeToken)
	if err != nil || token != upstreamToken || user.ID != "user_verify" {
		t.Fatalf("token=%q user=%#v err=%v", token, user, err)
	}
	if got := strings.Join(paths, ","); got != "/better-auth/one-time-token/apply,/better-auth/one-time-token/verify,/better-auth/get-session" {
		t.Fatalf("paths=%s", got)
	}
}

func TestAuthClientCompletesEmailAndTOTPWithoutExposingPendingCookie(t *testing.T) {
	const pendingCookie = "pending-two-factor-cookie-123456"
	const upstreamToken = "email-session-token-1234567890"
	var emailPassword string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/better-auth/sign-in/email":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			emailPassword, _ = body["password"].(string)
			http.SetCookie(writer, &http.Cookie{Name: "better-auth.two_factor", Value: pendingCookie, HttpOnly: true})
			_, _ = io.WriteString(writer, `{"twoFactorRedirect":true}`)
		case "/better-auth/two-factor/verify-totp":
			if request.Header.Get("Cookie") != "better-auth.two_factor="+pendingCookie {
				t.Fatalf("pending cookie=%q", request.Header.Get("Cookie"))
			}
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body["code"] != "123456" {
				t.Fatalf("body=%v err=%v", body, err)
			}
			http.SetCookie(writer, &http.Cookie{Name: "__Secure-better-auth.session_token", Value: upstreamToken, HttpOnly: true})
			_, _ = io.WriteString(writer, `{"ok":true}`)
		case "/better-auth/get-session":
			_, _ = io.WriteString(writer, `{"user":{"id":"user_email","name":"Email User","email":"user@example.invalid","image":null}}`)
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	client, err := folo.NewAuthClient(upstreamURL, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.SignInEmail(context.Background(), "user@example.invalid", "correct-password")
	if err != nil || !result.TwoFactorRequired || result.PendingCookie == "" || result.SessionToken != "" {
		t.Fatalf("email result=%#v err=%v", result, err)
	}
	completed, err := client.VerifyTOTP(context.Background(), result.PendingCookie, "123456")
	if err != nil || completed.SessionToken != upstreamToken || completed.User.ID != "user_email" {
		t.Fatalf("TOTP result=%#v err=%v", completed, err)
	}
	if emailPassword != "correct-password" {
		t.Fatal("email password was not sent to the expected fixed endpoint")
	}
}
