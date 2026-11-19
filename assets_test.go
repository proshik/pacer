package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"strings"
	"testing"

	"gorun/pkg/calculator"
	"gorun/pkg/http/rest"
)

// The real page carries the preview markers, so a shared link reaches a
// messenger with its plan in og:title. Without them the server would quietly
// serve the static tags and every preview would read "Pacer".
func TestSharedLinkPreviewShowsThePlan(t *testing.T) {
	handler, err := rest.NewHandler(true, "", "pacer.example.com", nil, calculator.NewService(), nil, nil, assets)
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?d=42195&t=3:44:20&l=ru", nil))

	want := `<meta property="og:title" content="Марафон за 3:44:20 — это 5:19 на километр">`
	if !strings.Contains(recorder.Body.String(), want) {
		t.Errorf("shared link page lacks %s", want)
	}
}

// localLink matches href and src values that point at a file next to the page:
// no scheme, no protocol-relative host, no fragment-only link.
var localLink = regexp.MustCompile(`(?:href|src)="([^"#:/][^"#:]*)"`)

// localURL matches the same kind of relative path inside a CSS url(), which is
// how the page reaches its font files.
var localURL = regexp.MustCompile(`url\("?([^"#:/)][^"#:)]*)"?\)`)

// The page reaches its manifest, icons, service worker and wasm by relative
// path, so each of them must be embedded and served with a type the browser
// accepts. Types are checked on the real handler over the real embedded files:
// the Alpine image has no mime.types, only Go's built-in table.
func TestEmbeddedPageAssetsAreServed(t *testing.T) {
	handler, err := rest.NewHandler(true, "", "pacer.example.com", nil, calculator.NewService(), nil, nil, assets)
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
		// Go's built-in table has no .woff2 and the Alpine image has no
		// mime.types; FileServer then sniffs the "wOF2" signature, so the
		// type does not depend on the host the image runs on.
		"/fonts/fira-sans-extra-condensed-800-cyrillic.woff2": {"font/woff2"},
		"/fonts/fira-sans-extra-condensed-800-latin.woff2":    {"font/woff2"},
		"/fonts/onest-cyrillic.woff2":                         {"font/woff2"},
		"/fonts/onest-latin.woff2":                            {"font/woff2"},
		"/fonts/jetbrains-mono-500-cyrillic.woff2":            {"font/woff2"},
		"/fonts/jetbrains-mono-500-latin.woff2":               {"font/woff2"},
	}

	page, err := fs.ReadFile(assets, "assets/index.html")
	if err != nil {
		t.Fatalf("read embedded page: %v", err)
	}
	links := append(localLink.FindAllStringSubmatch(string(page), -1), localURL.FindAllStringSubmatch(string(page), -1)...)
	for _, match := range links {
		// "./" (the logo's link home) is the page itself.
		if _, listed := want[path.Clean("/"+match[1])]; !listed {
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
