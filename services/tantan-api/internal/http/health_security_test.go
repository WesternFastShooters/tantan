package http

import (
	stdhttp "net/http"
	"strings"
	"testing"
)

func TestSecurityHeadersExplicitlyRestrictBrowserConnectionsToSelf(t *testing.T) {
	header := make(stdhttp.Header)
	setSecurityHeaders(header)
	policy := header.Get("Content-Security-Policy")
	if !strings.Contains(policy, "connect-src 'self'") {
		t.Fatalf("CSP does not explicitly restrict connect-src to self: %q", policy)
	}
}
