package folo

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

	"tantan.local/tantan-api/internal/session"
)

type AuthClient struct {
	upstream *url.URL
	client   *http.Client
}

func NewAuthClient(upstream *url.URL, client *http.Client) (*AuthClient, error) {
	if upstream == nil {
		return nil, errors.New("Folo auth upstream is required")
	}
	if !isAllowedAPIUpstream(upstream) {
		return nil, errors.New("Folo auth upstream must be https://api.folo.is or a loopback test server")
	}
	if client == nil {
		client = http.DefaultClient
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	upstreamCopy := *upstream
	return &AuthClient{upstream: &upstreamCopy, client: &clientCopy}, nil
}

func (client *AuthClient) ApplyOneTimeToken(ctx context.Context, oneTimeToken string) (string, session.User, error) {
	body, err := json.Marshal(map[string]string{"token": oneTimeToken})
	if err != nil {
		return "", session.User{}, errors.New("encode one-time token request")
	}
	response, responseBody, err := client.do(ctx, http.MethodPost, "/better-auth/one-time-token/apply", body, "")
	if err != nil {
		return "", session.User{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", session.User{}, fmt.Errorf("Folo one-time token apply status %d", response.StatusCode)
	}

	upstreamToken := sessionTokenFromCookies(response.Cookies())
	if upstreamToken == "" {
		var payload struct {
			Session struct {
				Token string `json:"token"`
			} `json:"session"`
		}
		if err := json.Unmarshal(responseBody, &payload); err == nil {
			upstreamToken = payload.Session.Token
		}
	}
	if !validCookieValue(upstreamToken) {
		return "", session.User{}, errors.New("Folo one-time token response did not create a session")
	}

	sessionResponse, sessionBody, err := client.do(ctx, http.MethodGet, "/better-auth/get-session", nil, upstreamToken)
	if err != nil {
		return "", session.User{}, err
	}
	if sessionResponse.StatusCode != http.StatusOK {
		_ = client.SignOut(ctx, upstreamToken)
		return "", session.User{}, fmt.Errorf("Folo session status %d", sessionResponse.StatusCode)
	}
	user, err := decodeSessionUser(sessionBody)
	if err != nil {
		_ = client.SignOut(ctx, upstreamToken)
		return "", session.User{}, err
	}
	return upstreamToken, user, nil
}

func (client *AuthClient) SignOut(ctx context.Context, upstreamToken string) error {
	response, _, err := client.do(ctx, http.MethodPost, "/better-auth/sign-out", nil, upstreamToken)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Folo sign-out status %d", response.StatusCode)
	}
	return nil
}

func (client *AuthClient) do(ctx context.Context, method, path string, body []byte, upstreamToken string) (*http.Response, []byte, error) {
	if upstreamToken != "" && !validCookieValue(upstreamToken) {
		return nil, nil, errors.New("invalid Folo session token")
	}
	target := *client.upstream
	target.Path = path
	target.RawPath = ""
	target.RawQuery = ""
	request, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, nil, errors.New("create Folo auth request")
	}
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	if upstreamToken != "" {
		request.Header.Set("Cookie", "__Secure-better-auth.session_token="+upstreamToken)
	}
	response, err := client.client.Do(request)
	if err != nil {
		return nil, nil, errors.New("Folo auth unavailable")
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024+1))
	if err != nil || len(responseBody) > 4*1024*1024 {
		return nil, nil, errors.New("invalid Folo auth response")
	}
	return response, responseBody, nil
}

func validCookieValue(value string) bool {
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

func sessionTokenFromCookies(cookies []*http.Cookie) string {
	for _, cookie := range cookies {
		if cookie.Name == "__Secure-better-auth.session_token" || cookie.Name == "better-auth.session_token" {
			return cookie.Value
		}
	}
	return ""
}

func decodeSessionUser(contents []byte) (session.User, error) {
	type responseUser struct {
		ID    string  `json:"id"`
		Name  string  `json:"name"`
		Email *string `json:"email"`
		Image *string `json:"image"`
	}
	var payload struct {
		User responseUser `json:"user"`
		Data *struct {
			User responseUser `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(contents, &payload); err != nil {
		return session.User{}, errors.New("decode Folo session response")
	}
	user := payload.User
	if user.ID == "" && payload.Data != nil {
		user = payload.Data.User
	}
	if strings.TrimSpace(user.ID) == "" || strings.TrimSpace(user.Name) == "" {
		return session.User{}, errors.New("Folo session response missing user")
	}
	return session.User{ID: user.ID, Name: user.Name, Email: user.Email, Image: user.Image}, nil
}
