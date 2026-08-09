package sync_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	syncer "tantan.local/tantan-api/internal/sync"
)

func TestHTTPSourceUsesLockedSDKRoutesAndDecodesResponses(t *testing.T) {
	const tokenCanary = "folo-session-canary-http-source"
	var mutex sync.Mutex
	requests := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Cookie") != "__Secure-better-auth.session_token="+tokenCanary {
			t.Error("Folo session cookie was not attached by the trusted source")
		}
		if request.Header.Get("Authorization") != "" {
			t.Error("unexpected Authorization header")
		}
		mutex.Lock()
		requests = append(requests, request.Method+" "+request.URL.RequestURI())
		mutex.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/subscriptions":
			_, _ = io.WriteString(writer, `{"code":0,"data":[{"feedId":"feed_1","view":2,"feeds":{"id":"feed_1","title":"Fixture feed","url":"https://feed.invalid/rss","image":"https://img.invalid/feed.png"}},{"listId":"list_1","view":0,"lists":{"id":"list_1"}}]}`)
		case "/entries":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("decode entry request: %v", err)
			}
			if body["limit"] != float64(100) || body["withContent"] != false || body["publishedBefore"] != "2026-08-09T10:00:00Z" {
				t.Errorf("entry request=%#v", body)
			}
			_, _ = io.WriteString(writer, `{"code":0,"data":[{"read":true,"view":2,"feeds":{"id":"feed_1","title":"Fixture feed","url":"https://feed.invalid/rss","image":null},"entries":{"id":"entry_1","title":"Fixture title","description":"Fixture description","author":"Author","url":"https://content.invalid/1","language":"zh-CN","media":[{"url":"https://img.invalid/1.png","type":"photo"}],"publishedAt":"2026-08-09T09:00:00Z"},"collections":{"createdAt":"2026-08-09T09:30:00Z"}}]}`)
		case "/entries/stream":
			writer.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = io.WriteString(writer, "{\"id\":\"entry_1\",\"content\":\"fixture content\"}\n")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	upstream, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	source, err := syncer.NewHTTPSource(syncer.HTTPSourceConfig{
		Upstream: upstream,
		Token: func(context.Context, string) (string, error) {
			return tokenCanary, nil
		},
	})
	if err != nil {
		t.Fatalf("create HTTP source: %v", err)
	}
	ctx := context.Background()
	feeds, err := source.ListSubscriptions(ctx, "user_1")
	if err != nil || len(feeds) != 1 || feeds[0].ID != "feed_1" || feeds[0].View != 2 {
		t.Fatalf("feeds=%#v err=%v", feeds, err)
	}
	before := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	entries, err := source.ListEntries(ctx, "user_1", syncer.PageRequest{Limit: 100, PublishedBefore: &before})
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries=%#v err=%v", entries, err)
	}
	entry := entries[0]
	if entry.ID != "entry_1" || entry.Feed.ID != "feed_1" || !entry.Read || entry.CollectedAt == nil || entry.View != 2 || !json.Valid(entry.MediaJSON) {
		t.Fatalf("decoded entry=%#v", entry)
	}
	stream, err := source.StreamContents(ctx, "user_1", []string{"entry_1"})
	if err != nil {
		t.Fatalf("content stream: %v", err)
	}
	contents, readErr := io.ReadAll(stream)
	closeErr := stream.Close()
	if readErr != nil || closeErr != nil || !strings.Contains(string(contents), "fixture content") {
		t.Fatalf("content=%q readErr=%v closeErr=%v", contents, readErr, closeErr)
	}
	if len(requests) != 3 || requests[0] != "GET /subscriptions" || requests[1] != "POST /entries" || requests[2] != "POST /entries/stream" {
		t.Fatalf("requests=%v", requests)
	}
}

func TestHTTPSourceRejectsUntrustedUpstreamAndBoundsFailures(t *testing.T) {
	untrusted, _ := url.Parse("https://attacker.invalid")
	if _, err := syncer.NewHTTPSource(syncer.HTTPSourceConfig{Upstream: untrusted, Token: func(context.Context, string) (string, error) { return "token", nil }}); err == nil {
		t.Fatal("untrusted Folo upstream was accepted")
	}

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		if strings.Contains(request.Header.Get("Cookie"), "\r") {
			t.Fatal("unsafe token reached upstream")
		}
		http.Error(writer, "private upstream response must not be reflected", http.StatusTooManyRequests)
	}))
	defer server.Close()
	upstream, _ := url.Parse(server.URL)
	source, err := syncer.NewHTTPSource(syncer.HTTPSourceConfig{
		Upstream: upstream,
		Token: func(context.Context, string) (string, error) {
			return "unsafe\r\ntoken", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.ListSubscriptions(context.Background(), "user_1"); err == nil || calls != 0 || strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("invalid-token err=%v calls=%d", err, calls)
	}

	source, err = syncer.NewHTTPSource(syncer.HTTPSourceConfig{
		Upstream: upstream,
		Token: func(context.Context, string) (string, error) {
			return "safe-token", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.ListSubscriptions(context.Background(), "user_1")
	temporary, ok := err.(interface{ Temporary() bool })
	if err == nil || !ok || !temporary.Temporary() || strings.Contains(err.Error(), "private upstream") {
		t.Fatalf("429 error=%v", err)
	}
}
