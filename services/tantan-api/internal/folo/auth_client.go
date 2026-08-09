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

var (
	ErrAuthInvalid     = errors.New("Folo authentication is invalid")
	ErrAuthUnavailable = errors.New("Folo authentication is unavailable")
)

type LoginResult struct {
	SessionToken      string
	User              session.User
	TwoFactorRequired bool
	PendingCookie     string
}

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
	if response.StatusCode == http.StatusNotFound {
		response, responseBody, err = client.do(ctx, http.MethodPost, "/better-auth/one-time-token/verify", body, "")
		if err != nil {
			return "", session.User{}, err
		}
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		result, resultErr := client.completeLogin(ctx, response, responseBody)
		if resultErr != nil {
			return "", session.User{}, resultErr
		}
		return result.SessionToken, result.User, nil
	}
	if validCookieValue(oneTimeToken) {
		user, sessionErr := client.sessionUser(ctx, oneTimeToken)
		if sessionErr == nil {
			return oneTimeToken, user, nil
		}
	}
	return "", session.User{}, ErrAuthInvalid
}

func (client *AuthClient) SignInEmail(ctx context.Context, email, password string) (LoginResult, error) {
	body, err := json.Marshal(map[string]any{"email": email, "password": password, "rememberMe": true})
	if err != nil {
		return LoginResult{}, errors.New("encode Folo email login request")
	}
	response, responseBody, err := client.do(ctx, http.MethodPost, "/better-auth/sign-in/email", body, "")
	if err != nil {
		return LoginResult{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode >= 400 && response.StatusCode < 500 {
			return LoginResult{}, ErrAuthInvalid
		}
		return LoginResult{}, ErrAuthUnavailable
	}
	var payload struct {
		TwoFactorRedirect bool `json:"twoFactorRedirect"`
	}
	_ = json.Unmarshal(responseBody, &payload)
	if payload.TwoFactorRedirect {
		pendingCookie := pendingCookieHeader(response.Cookies())
		if pendingCookie == "" {
			return LoginResult{}, ErrAuthUnavailable
		}
		return LoginResult{TwoFactorRequired: true, PendingCookie: pendingCookie}, nil
	}
	return client.completeLogin(ctx, response, responseBody)
}

func (client *AuthClient) VerifyTOTP(ctx context.Context, pendingCookie, code string) (LoginResult, error) {
	if !validCookieHeader(pendingCookie) {
		return LoginResult{}, ErrAuthInvalid
	}
	body, err := json.Marshal(map[string]any{"code": code, "trustDevice": true})
	if err != nil {
		return LoginResult{}, errors.New("encode Folo TOTP request")
	}
	response, responseBody, err := client.doWithCookieHeader(ctx, http.MethodPost, "/better-auth/two-factor/verify-totp", body, pendingCookie)
	if err != nil {
		return LoginResult{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode >= 400 && response.StatusCode < 500 {
			return LoginResult{}, ErrAuthInvalid
		}
		return LoginResult{}, ErrAuthUnavailable
	}
	return client.completeLogin(ctx, response, responseBody)
}

func (client *AuthClient) completeLogin(ctx context.Context, response *http.Response, responseBody []byte) (LoginResult, error) {
	upstreamToken := sessionTokenFromCookies(response.Cookies())
	if upstreamToken == "" {
		var payload struct {
			Session struct {
				Token string `json:"token"`
			} `json:"session"`
			Token string `json:"token"`
		}
		if err := json.Unmarshal(responseBody, &payload); err == nil {
			upstreamToken = payload.Session.Token
			if upstreamToken == "" {
				upstreamToken = payload.Token
			}
		}
	}
	if !validCookieValue(upstreamToken) {
		return LoginResult{}, ErrAuthUnavailable
	}
	user, err := client.sessionUser(ctx, upstreamToken)
	if err != nil {
		_ = client.SignOut(ctx, upstreamToken)
		return LoginResult{}, err
	}
	return LoginResult{SessionToken: upstreamToken, User: user}, nil
}

func (client *AuthClient) sessionUser(ctx context.Context, upstreamToken string) (session.User, error) {

	sessionResponse, sessionBody, err := client.do(ctx, http.MethodGet, "/better-auth/get-session", nil, upstreamToken)
	if err != nil {
		return session.User{}, err
	}
	if sessionResponse.StatusCode != http.StatusOK {
		return session.User{}, ErrAuthInvalid
	}
	user, err := decodeSessionUser(sessionBody)
	if err != nil {
		return session.User{}, err
	}
	return user, nil
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
	cookieHeader := ""
	if upstreamToken != "" {
		cookieHeader = "__Secure-better-auth.session_token=" + upstreamToken
	}
	return client.doWithCookieHeader(ctx, method, path, body, cookieHeader)
}

func (client *AuthClient) doWithCookieHeader(ctx context.Context, method, path string, body []byte, cookieHeader string) (*http.Response, []byte, error) {
	if cookieHeader != "" && !validCookieHeader(cookieHeader) {
		return nil, nil, errors.New("invalid Folo auth cookie header")
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
	if cookieHeader != "" {
		request.Header.Set("Cookie", cookieHeader)
	}
	response, err := client.client.Do(request)
	if err != nil {
		return nil, nil, ErrAuthUnavailable
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024+1))
	if err != nil || len(responseBody) > 4*1024*1024 {
		return nil, nil, errors.New("invalid Folo auth response")
	}
	return response, responseBody, nil
}

func pendingCookieHeader(cookies []*http.Cookie) string {
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie == nil || !strings.Contains(cookie.Name, "two_factor") || !validCookieName(cookie.Name) || !validCookieValue(cookie.Value) {
			continue
		}
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	result := strings.Join(parts, "; ")
	if !validCookieHeader(result) {
		return ""
	}
	return result
}

func validCookieName(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || strings.ContainsRune("!#$%&'*+-.^_`|~", character)) {
			return false
		}
	}
	return true
}

func validCookieHeader(value string) bool {
	if len(value) < 1 || len(value) > 8192 || strings.ContainsAny(value, "\r\n\x00") {
		return false
	}
	return true
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
