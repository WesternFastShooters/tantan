package ai

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func newProviderTransport() (*http.Transport, error) {
	proxyURL, err := providerProxyFromEnvironment(os.Getenv)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          4,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: providerTimeout,
	}
	if proxyURL != nil {
		// The proxy receives only an HTTPS CONNECT target. Provider credentials are
		// added after the end-to-end TLS tunnel is established.
		transport.Proxy = http.ProxyURL(proxyURL)
		transport.DialContext = (&net.Dialer{}).DialContext
		return transport, nil
	}

	safe := newSafeDialer()
	transport.Proxy = nil
	transport.DialContext = safe.DialContext
	return transport, nil
}

func providerProxyFromEnvironment(getenv func(string) string) (*url.URL, error) {
	if getenv == nil {
		return nil, errors.New("AI provider proxy environment is unavailable")
	}
	raw := strings.TrimSpace(getenv("HTTPS_PROXY"))
	if raw == "" {
		raw = strings.TrimSpace(getenv("https_proxy"))
	}
	if raw == "" {
		return nil, nil
	}

	proxyURL, err := url.Parse(raw)
	if err != nil || (proxyURL.Scheme != "http" && proxyURL.Scheme != "https") || proxyURL.User != nil || proxyURL.RawQuery != "" || proxyURL.Fragment != "" || (proxyURL.Path != "" && proxyURL.Path != "/") {
		return nil, errors.New("AI provider proxy must be a loopback HTTP proxy")
	}
	host := net.ParseIP(proxyURL.Hostname())
	if host == nil || !host.IsLoopback() || proxyURL.Port() == "" {
		return nil, errors.New("AI provider proxy must be a loopback HTTP proxy")
	}
	return proxyURL, nil
}
