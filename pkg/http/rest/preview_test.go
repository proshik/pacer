package rest

import (
	"html"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"gorun/pkg/calculator"
)

// previewPage mirrors the head of assets/index.html: the block between the
// markers holds the static tags a plain file server would serve.
const previewPage = `<!doctype html><html><head>
    <!-- og:start -->
    <meta property="og:title" content="Pacer">
    <!-- og:end -->
</head><body><h1 id="planLine"></h1></body></html>`

func newPreviewHandler(t *testing.T, page string) *http.ServeMux {
	t.Helper()

	assets := fstest.MapFS{"assets/index.html": &fstest.MapFile{Data: []byte(page)}}
	handler, err := NewHandler(true, "token", "pacer.example.com", nil, calculator.NewService(), nil, assets)
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}

	return handler
}

func getPage(t *testing.T, handler http.Handler, path string, acceptLanguage string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, path, nil)
	if acceptLanguage != "" {
		request.Header.Set("Accept-Language", acceptLanguage)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", path, recorder.Code)
	}

	return recorder
}

var metaTag = regexp.MustCompile(`<meta (?:property|name)="([^"]+)" content="([^"]*)">`)

func metaContent(body string) map[string]string {
	tags := map[string]string{}
	for _, match := range metaTag.FindAllStringSubmatch(body, -1) {
		tags[match[1]] = html.UnescapeString(match[2])
	}

	return tags
}

// A shared link reaches a messenger with the plan it carries, phrased the way
// the page's own header phrases it.
func TestPreviewCarriesThePlanFromTheLink(t *testing.T) {
	handler := newPreviewHandler(t, previewPage)

	tests := []struct {
		path       string
		wantTitle  string
		wantURL    string
		wantLocale string
	}{
		{
			path:       "/?d=42195&t=3:44:20&l=ru",
			wantTitle:  "Марафон за 3:44:20 — это 5:19 на километр",
			wantURL:    "https://pacer.example.com/?d=42195&t=3:44:20",
			wantLocale: "ru_RU",
		},
		{
			path:       "/?d=42195&t=3:44:20&l=en",
			wantTitle:  "Marathon in 3:44:20 — that's 5:19 per kilometer",
			wantURL:    "https://pacer.example.com/?d=42195&t=3:44:20",
			wantLocale: "en_US",
		},
		{
			path:       "/?d=21097&t=1:38:48&l=ru",
			wantTitle:  "Полумарафон за 1:38:48 — это 4:41 на километр",
			wantURL:    "https://pacer.example.com/?d=21097&t=1:38:48",
			wantLocale: "ru_RU",
		},
		{
			path:       "/?d=10000&t=50:00&l=ru",
			wantTitle:  "10 км за 50:00 — это 5:00 на километр",
			wantURL:    "https://pacer.example.com/?d=10000&t=0:50:00",
			wantLocale: "ru_RU",
		},
		{
			// a bare number is total seconds, as on the page
			path:       "/?d=5500&t=1650&l=en",
			wantTitle:  "5.5 km in 27:30 — that's 5:00 per kilometer",
			wantURL:    "https://pacer.example.com/?d=5500&t=0:27:30",
			wantLocale: "en_US",
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			tags := metaContent(getPage(t, handler, tt.path, "").Body.String())

			if got := tags["og:title"]; got != tt.wantTitle {
				t.Errorf("og:title = %q, want %q", got, tt.wantTitle)
			}
			if got := tags["og:url"]; got != tt.wantURL {
				t.Errorf("og:url = %q, want %q", got, tt.wantURL)
			}
			if got := tags["og:locale"]; got != tt.wantLocale {
				t.Errorf("og:locale = %q, want %q", got, tt.wantLocale)
			}
			if got, want := tags["og:image"], "https://pacer.example.com/icon-512.png"; got != want {
				t.Errorf("og:image = %q, want %q", got, want)
			}
		})
	}
}

// Crawlers rarely say which language they want, so the link names the
// sharer's language in l; without it Accept-Language decides, and Russian is
// the default, as every link shared before l existed was Russian.
func TestPreviewLanguage(t *testing.T) {
	handler := newPreviewHandler(t, previewPage)

	tests := []struct {
		name           string
		query          string
		acceptLanguage string
		wantTitle      string
	}{
		{name: "l wins over the header", query: "&l=ru", acceptLanguage: "en-US,en", wantTitle: "Марафон за 3:44:20 — это 5:19 на километр"},
		{name: "header without l", acceptLanguage: "en-US,en;q=0.9", wantTitle: "Marathon in 3:44:20 — that's 5:19 per kilometer"},
		{name: "first supported language in the header", acceptLanguage: "de-DE,de;q=0.9,en;q=0.8", wantTitle: "Marathon in 3:44:20 — that's 5:19 per kilometer"},
		{name: "unsupported header falls back to Russian", acceptLanguage: "de-DE,de", wantTitle: "Марафон за 3:44:20 — это 5:19 на километр"},
		{name: "no hint at all is Russian", wantTitle: "Марафон за 3:44:20 — это 5:19 на километр"},
		{name: "unknown l is ignored", query: "&l=xx", acceptLanguage: "en", wantTitle: "Marathon in 3:44:20 — that's 5:19 per kilometer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := getPage(t, handler, "/?d=42195&t=3:44:20"+tt.query, tt.acceptLanguage)

			if got := metaContent(recorder.Body.String())["og:title"]; got != tt.wantTitle {
				t.Errorf("og:title = %q, want %q", got, tt.wantTitle)
			}
			if vary := strings.Join(recorder.Header().Values("Vary"), ","); !strings.Contains(vary, "Accept-Language") {
				t.Errorf("Vary = %q, want it to name Accept-Language", vary)
			}
		})
	}
}

// Anything that is not a complete, valid plan keeps the site's own title, and
// nothing from the query string reaches the markup.
func TestPreviewWithoutAPlanKeepsTheDefaults(t *testing.T) {
	handler := newPreviewHandler(t, previewPage)

	for _, path := range []string{
		"/",
		"/?d=42195",
		"/?t=3:44:20",
		"/?d=abc&t=3:44:20",
		"/?d=0&t=3:44:20",
		"/?d=42195&t=0:00:00",
		"/?d=42195&t=3:75:00",
		"/?d=42195&t=3:44:20%22%3E%3Cscript%3Ealert(1)%3C/script%3E",
		"/?d=42195%22%3E%3Cscript%3E&t=3:44:20",
	} {
		t.Run(path, func(t *testing.T) {
			recorder := getPage(t, handler, path, "")
			body := recorder.Body.String()
			tags := metaContent(body)

			if got := tags["og:title"]; got != "Pacer" {
				t.Errorf("og:title = %q, want the default %q", got, "Pacer")
			}
			if got, want := tags["og:url"], "https://pacer.example.com/"; got != want {
				t.Errorf("og:url = %q, want %q", got, want)
			}
			if strings.Contains(body, "<script>") {
				t.Errorf("query string reached the markup: %s", body)
			}
			if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
				t.Errorf("Content-Type = %q, want text/html", got)
			}
		})
	}
}

// A page without the markers is served byte for byte, so the preview can never
// damage a page it does not understand.
func TestPageWithoutPreviewMarkersIsServedUnchanged(t *testing.T) {
	const page = `<!doctype html><html><head><title>x</title></head><body>plain</body></html>`
	handler := newPreviewHandler(t, page)

	if got := getPage(t, handler, "/?d=42195&t=3:44:20", "").Body.String(); got != page {
		t.Errorf("body = %q, want the page unchanged", got)
	}
}
