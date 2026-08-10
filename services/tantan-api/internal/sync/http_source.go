package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxFoloJSONResponseBytes = 16 * 1024 * 1024
	defaultFoloTimeout       = 30 * time.Second
	maxFoloContentBatchIDs   = 30
)

type TokenProvider func(ctx context.Context, userID string) (string, error)

type HTTPSourceConfig struct {
	Upstream *url.URL
	Client   *http.Client
	Token    TokenProvider
}

type HTTPSource struct {
	upstream *url.URL
	client   *http.Client
	token    TokenProvider
}

type upstreamError struct {
	status    int
	retryable bool
}

func (failure upstreamError) Error() string {
	if failure.status == 0 {
		return "Folo is unavailable"
	}
	return fmt.Sprintf("Folo request failed with status %d", failure.status)
}

func (failure upstreamError) Temporary() bool {
	return failure.retryable
}

func NewHTTPSource(config HTTPSourceConfig) (*HTTPSource, error) {
	if !allowedSyncUpstream(config.Upstream) || config.Token == nil {
		return nil, errors.New("trusted Folo upstream and token provider are required")
	}
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: defaultFoloTimeout}
	}
	clientCopy := *client
	if clientCopy.Timeout <= 0 {
		clientCopy.Timeout = defaultFoloTimeout
	}
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	upstream := *config.Upstream
	return &HTTPSource{upstream: &upstream, client: &clientCopy, token: config.Token}, nil
}

func (source *HTTPSource) ListSubscriptions(ctx context.Context, userID string) ([]RemoteFeed, error) {
	var payload struct {
		Code int `json:"code"`
		Data []struct {
			FeedID string `json:"feedId"`
			View   int    `json:"view"`
			Feeds  *struct {
				ID    string  `json:"id"`
				Title *string `json:"title"`
				URL   string  `json:"url"`
				Image *string `json:"image"`
			} `json:"feeds"`
		} `json:"data"`
	}
	if err := source.doJSON(ctx, userID, http.MethodGet, "/subscriptions", nil, &payload); err != nil {
		return nil, err
	}
	if payload.Code != 0 || payload.Data == nil {
		return nil, errors.New("Folo subscriptions response is invalid")
	}
	feeds := make([]RemoteFeed, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		if item.Feeds == nil {
			continue
		}
		feedID := strings.TrimSpace(item.Feeds.ID)
		if feedID == "" || (item.FeedID != "" && item.FeedID != feedID) || strings.TrimSpace(item.Feeds.URL) == "" {
			return nil, errors.New("Folo subscriptions response contains an invalid feed")
		}
		if _, duplicate := seen[feedID]; duplicate {
			continue
		}
		seen[feedID] = struct{}{}
		feeds = append(feeds, RemoteFeed{
			ID:    feedID,
			Title: dereferenceString(item.Feeds.Title),
			URL:   item.Feeds.URL,
			Image: item.Feeds.Image,
			View:  item.View,
		})
	}
	return feeds, nil
}

func (source *HTTPSource) ListEntries(ctx context.Context, userID string, request PageRequest) ([]RemoteEntry, error) {
	if request.Limit < 1 || request.Limit > PageSize {
		return nil, errors.New("Folo entry page limit must be between 1 and 100")
	}
	body := struct {
		Limit           int     `json:"limit"`
		PublishedAfter  *string `json:"publishedAfter,omitempty"`
		PublishedBefore *string `json:"publishedBefore,omitempty"`
		WithContent     bool    `json:"withContent"`
	}{Limit: request.Limit}
	if request.PublishedAfter != nil {
		value := request.PublishedAfter.UTC().Format(time.RFC3339Nano)
		body.PublishedAfter = &value
	}
	if request.PublishedBefore != nil {
		value := request.PublishedBefore.UTC().Format(time.RFC3339Nano)
		body.PublishedBefore = &value
	}
	var payload struct {
		Code int             `json:"code"`
		Data []httpEntryItem `json:"data"`
	}
	if err := source.doJSON(ctx, userID, http.MethodPost, "/entries", body, &payload); err != nil {
		return nil, err
	}
	if payload.Code != 0 || payload.Data == nil || len(payload.Data) > request.Limit {
		return nil, errors.New("Folo entries response is invalid")
	}
	entries := make([]RemoteEntry, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		entry, err := item.remoteEntry()
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[entry.ID]; duplicate {
			return nil, errors.New("Folo entries response contains duplicate IDs")
		}
		seen[entry.ID] = struct{}{}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (source *HTTPSource) StreamContents(ctx context.Context, userID string, entryIDs []string) (io.ReadCloser, error) {
	if len(entryIDs) < 1 || len(entryIDs) > 50 {
		return nil, errors.New("Folo content request requires 1 to 50 entry IDs")
	}
	seen := make(map[string]struct{}, len(entryIDs))
	for _, entryID := range entryIDs {
		if strings.TrimSpace(entryID) == "" {
			return nil, errors.New("Folo content request contains an invalid entry ID")
		}
		if _, duplicate := seen[entryID]; duplicate {
			return nil, errors.New("Folo content request contains duplicate entry IDs")
		}
		seen[entryID] = struct{}{}
	}
	readers := make([]io.Reader, 0, (len(entryIDs)+maxFoloContentBatchIDs-1)/maxFoloContentBatchIDs)
	closers := make([]io.Closer, 0, cap(readers))
	for start := 0; start < len(entryIDs); start += maxFoloContentBatchIDs {
		end := min(start+maxFoloContentBatchIDs, len(entryIDs))
		stream, err := source.streamContentBatch(ctx, userID, entryIDs[start:end])
		if err != nil {
			closeAll(closers)
			return nil, err
		}
		readers = append(readers, stream)
		closers = append(closers, stream)
	}
	return &multiReadCloser{Reader: io.MultiReader(readers...), closers: closers}, nil
}

func (source *HTTPSource) streamContentBatch(ctx context.Context, userID string, entryIDs []string) (io.ReadCloser, error) {
	body, err := json.Marshal(struct {
		IDs []string `json:"ids"`
	}{IDs: entryIDs})
	if err != nil {
		return nil, errors.New("encode Folo content request")
	}
	response, err := source.do(ctx, userID, http.MethodPost, "/entries/stream", body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_ = response.Body.Close()
		return nil, statusError(response.StatusCode)
	}
	return response.Body, nil
}

type multiReadCloser struct {
	io.Reader
	closers []io.Closer
}

func (stream *multiReadCloser) Close() error {
	var closeErr error
	for _, closer := range stream.closers {
		if err := closer.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func closeAll(closers []io.Closer) {
	for _, closer := range closers {
		_ = closer.Close()
	}
}

type httpEntryItem struct {
	Read  bool `json:"read"`
	View  int  `json:"view"`
	Feeds struct {
		ID    string  `json:"id"`
		Title *string `json:"title"`
		URL   string  `json:"url"`
		Image *string `json:"image"`
	} `json:"feeds"`
	Entries struct {
		ID          string          `json:"id"`
		Title       *string         `json:"title"`
		Description *string         `json:"description"`
		Author      *string         `json:"author"`
		URL         *string         `json:"url"`
		Language    *string         `json:"language"`
		Media       json.RawMessage `json:"media"`
		PublishedAt string          `json:"publishedAt"`
	} `json:"entries"`
	Collections *struct {
		CreatedAt string `json:"createdAt"`
	} `json:"collections"`
}

func (item httpEntryItem) remoteEntry() (RemoteEntry, error) {
	entryID := strings.TrimSpace(item.Entries.ID)
	feedID := strings.TrimSpace(item.Feeds.ID)
	if entryID == "" || feedID == "" || strings.TrimSpace(item.Feeds.URL) == "" {
		return RemoteEntry{}, errors.New("Folo entries response contains an invalid entry")
	}
	publishedAt, err := time.Parse(time.RFC3339Nano, item.Entries.PublishedAt)
	if err != nil {
		return RemoteEntry{}, errors.New("Folo entry has an invalid published timestamp")
	}
	media := item.Entries.Media
	if len(media) == 0 || string(media) == "null" {
		media = json.RawMessage("[]")
	}
	if !json.Valid(media) {
		return RemoteEntry{}, errors.New("Folo entry has invalid media")
	}
	var collectedAt *time.Time
	if item.Collections != nil {
		parsed, err := time.Parse(time.RFC3339Nano, item.Collections.CreatedAt)
		if err != nil {
			return RemoteEntry{}, errors.New("Folo entry has an invalid collection timestamp")
		}
		parsed = parsed.UTC()
		collectedAt = &parsed
	}
	return RemoteEntry{
		ID: entryID,
		Feed: RemoteFeed{
			ID:    feedID,
			Title: dereferenceString(item.Feeds.Title),
			URL:   item.Feeds.URL,
			Image: item.Feeds.Image,
			View:  item.View,
		},
		View:        item.View,
		Title:       dereferenceString(item.Entries.Title),
		Description: dereferenceString(item.Entries.Description),
		Author:      dereferenceString(item.Entries.Author),
		URL:         dereferenceString(item.Entries.URL),
		Language:    dereferenceString(item.Entries.Language),
		MediaJSON:   append([]byte(nil), media...),
		PublishedAt: publishedAt.UTC(),
		Read:        item.Read,
		CollectedAt: collectedAt,
	}, nil
}

func (source *HTTPSource) doJSON(ctx context.Context, userID, method, path string, body any, target any) error {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return errors.New("encode Folo request")
		}
	}
	response, err := source.do(ctx, userID, method, path, encoded)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return statusError(response.StatusCode)
	}
	contents, err := readLimited(response.Body, maxFoloJSONResponseBytes)
	if err != nil {
		return errors.New("Folo response exceeds the safe limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	if err := decoder.Decode(target); err != nil {
		return errors.New("Folo response is invalid JSON")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("Folo response contains trailing data")
	}
	return nil
}

func (source *HTTPSource) do(ctx context.Context, userID, method, path string, body []byte) (*http.Response, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("Folo sync user is required")
	}
	token, err := source.token(ctx, userID)
	if err != nil || !safeCookieValue(token) {
		return nil, errors.New("Folo session is unavailable")
	}
	target := *source.upstream
	target.Path = path
	target.RawPath = ""
	target.RawQuery = ""
	request, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("create Folo request")
	}
	request.Header.Set("Accept", "application/json, application/x-ndjson")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Cookie", "__Secure-better-auth.session_token="+token)
	response, err := source.client.Do(request)
	if err != nil {
		return nil, upstreamError{retryable: true}
	}
	return response, nil
}

func statusError(status int) error {
	return upstreamError{status: status, retryable: status == http.StatusTooManyRequests || status >= 500}
}

func allowedSyncUpstream(value *url.URL) bool {
	if value == nil || value.User != nil || value.RawQuery != "" || value.Fragment != "" || (value.Path != "" && value.Path != "/") {
		return false
	}
	official := value.Scheme == "https" && value.Host == "api.folo.is"
	loopback := value.Scheme == "http" && (value.Hostname() == "127.0.0.1" || value.Hostname() == "localhost")
	return official || loopback
}

func safeCookieValue(value string) bool {
	if len(value) < 1 || len(value) > 4096 {
		return false
	}
	for index := range len(value) {
		character := value[index]
		if character < 0x21 || character > 0x7e || character == '"' || character == ',' || character == ';' || character == '\\' {
			return false
		}
	}
	return true
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	contents, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(contents)) > limit {
		return nil, errors.New("size limit exceeded")
	}
	return contents, nil
}

func dereferenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
