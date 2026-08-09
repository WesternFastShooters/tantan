package ai

import "testing"

func TestProviderProxyAllowsOnlyExplicitLoopbackHTTPProxy(t *testing.T) {
	tests := []struct {
		name      string
		values    map[string]string
		wantProxy string
		wantError bool
	}{
		{name: "unset", values: map[string]string{}},
		{name: "uppercase", values: map[string]string{"HTTPS_PROXY": "http://127.0.0.1:7897"}, wantProxy: "http://127.0.0.1:7897"},
		{name: "lowercase IPv6", values: map[string]string{"https_proxy": "http://[::1]:7897"}, wantProxy: "http://[::1]:7897"},
		{name: "remote", values: map[string]string{"HTTPS_PROXY": "http://203.0.113.5:8080"}, wantError: true},
		{name: "hostname", values: map[string]string{"HTTPS_PROXY": "http://localhost:7897"}, wantError: true},
		{name: "credentials", values: map[string]string{"HTTPS_PROXY": "http://user:secret@127.0.0.1:7897"}, wantError: true},
		{name: "path", values: map[string]string{"HTTPS_PROXY": "http://127.0.0.1:7897/proxy"}, wantError: true},
		{name: "unsupported scheme", values: map[string]string{"HTTPS_PROXY": "socks5://127.0.0.1:7897"}, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proxyURL, err := providerProxyFromEnvironment(func(name string) string {
				return test.values[name]
			})
			if test.wantError {
				if err == nil {
					t.Fatal("unsafe proxy was accepted")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve Provider proxy: %v", err)
			}
			if test.wantProxy == "" {
				if proxyURL != nil {
					t.Fatalf("proxy=%q, want none", proxyURL)
				}
				return
			}
			if proxyURL == nil || proxyURL.String() != test.wantProxy {
				t.Fatalf("proxy=%v, want %q", proxyURL, test.wantProxy)
			}
		})
	}
}
