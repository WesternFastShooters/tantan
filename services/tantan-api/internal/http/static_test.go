package http_test

import (
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	localhttp "tantan.local/tantan-api/internal/http"
)

func TestSPAHandlerServesAssetsFallbackAndRejectsUnsafeFiles(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("<main>mobile shell</main>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "assets", "app.0123456789abcdef.js"), []byte("export{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler, err := localhttp.NewSPAHandler(directory)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		path         string
		status       int
		body         string
		cacheControl string
	}{
		{path: "/", status: stdhttp.StatusOK, body: "mobile shell", cacheControl: "no-store"},
		{path: "/settings", status: stdhttp.StatusOK, body: "mobile shell", cacheControl: "no-store"},
		{path: "/assets/app.0123456789abcdef.js", status: stdhttp.StatusOK, body: "export{}", cacheControl: "immutable"},
		{path: "/assets/missing.js", status: stdhttp.StatusNotFound},
		{path: "/unsafe\\path", status: stdhttp.StatusNotFound},
	} {
		request := httptest.NewRequest(stdhttp.MethodGet, "http://127.0.0.1:3000"+test.path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.status || test.body != "" && !strings.Contains(response.Body.String(), test.body) || test.cacheControl != "" && !strings.Contains(response.Header().Get("Cache-Control"), test.cacheControl) {
			t.Fatalf("%s status=%d body=%q cache=%q", test.path, response.Code, response.Body.String(), response.Header().Get("Cache-Control"))
		}
	}

	symlink := filepath.Join(directory, "unsafe-link")
	if err := os.Symlink(filepath.Join(directory, "index.html"), symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := localhttp.NewSPAHandler(directory); err == nil {
		t.Fatal("accepted symlink in static assets")
	}
}
