package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"gorun/pkg/calculator"
	"gorun/pkg/http/rest"
)

// localLink matches href and src values that point at a file next to the page:
// no scheme, no protocol-relative host, no fragment-only link.
var localLink = regexp.MustCompile(`(?:href|src)="([^"#:/][^"#:]*)"`)

// The page reaches its manifest, icons, service worker and wasm by relative
// path, so each of them must be embedded and served with a type the browser
// accepts. Types are checked on the real handler over the real embedded files:
// the Alpine image has no mime.types, only Go's built-in table.
func TestEmbeddedPageAssetsAreServed(t *testing.T) {
	handler, err := rest.NewHandler(true, "", nil, calculator.NewService(), nil, assets)
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}

	javascript := []string{"text/javascript", "application/javascript"}
	want := map[string][]string{
		"/":                     {"text/html"},
		"/manifest.json":        {"application/json", "application/manifest+json"},
		"/sw.js":                javascript,
		"/wasm_exec.js":         javascript,
		"/json.wasm":            {"application/wasm"},
		"/icon.svg":             {"image/svg+xml"},
		"/icon-192.png":         {"image/png"},
		"/icon-512.png":         {"image/png"},
		"/apple-touch-icon.png": {"image/png"},
	}

	page, err := fs.ReadFile(assets, "assets/index.html")
	if err != nil {
		t.Fatalf("read embedded page: %v", err)
	}
	for _, match := range localLink.FindAllStringSubmatch(string(page), -1) {
		if _, listed := want["/"+match[1]]; !listed {
			t.Errorf("index.html links %q, which this test does not check", match[1])
		}
	}

	for path, types := range want {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))

			if recorder.Code != http.StatusOK {
				t.Fatalf("GET %s = %d, want 200", path, recorder.Code)
			}

			got := recorder.Header().Get("Content-Type")
			for _, prefix := range types {
				if strings.HasPrefix(got, prefix) {
					return
				}
			}
			t.Errorf("GET %s Content-Type = %q, want one of %v", path, got, types)
		})
	}
}
