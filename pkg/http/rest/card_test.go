package rest

import (
	"bytes"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorun/pkg/card"
)

func getCard(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))

	return recorder
}

// The card is the plan as a picture, for a chat or a link preview.
func TestCardImage(t *testing.T) {
	handler := newTestHandler(t)

	recorder := getCard(t, handler, "/api/v1/card.png?d=42195&t=3:44:20&l=ru")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %.120q)", recorder.Code, recorder.Body.String())
	}

	if got := recorder.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", got)
	}

	picture, err := png.Decode(bytes.NewReader(recorder.Body.Bytes()))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if picture.Bounds().Dx() != card.Width || picture.Bounds().Dy() != card.Height {
		t.Errorf("size = %dx%d, want %dx%d", picture.Bounds().Dx(), picture.Bounds().Dy(), card.Width, card.Height)
	}
}

// The language follows the link, as it does for the preview.
func TestCardImageFollowsTheLanguage(t *testing.T) {
	handler := newTestHandler(t)

	russian := getCard(t, handler, "/api/v1/card.png?d=42195&t=3:44:20&l=ru").Body.Bytes()
	english := getCard(t, handler, "/api/v1/card.png?d=42195&t=3:44:20&l=en").Body.Bytes()

	if bytes.Equal(russian, english) {
		t.Error("the English card is the same picture as the Russian one")
	}
}

func TestCardImageRejectsBadInput(t *testing.T) {
	handler := newTestHandler(t)

	for _, path := range []string{
		"/api/v1/card.png",
		"/api/v1/card.png?d=42195",
		"/api/v1/card.png?t=3:44:20",
		"/api/v1/card.png?d=0&t=3:44:20",
		"/api/v1/card.png?d=abc&t=3:44:20",
		"/api/v1/card.png?d=42195&t=3:75:00",
	} {
		t.Run(path, func(t *testing.T) {
			recorder := getCard(t, handler, path)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", recorder.Code)
			}

			var problem errorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
				t.Fatalf("decode error body %q: %v", recorder.Body.String(), err)
			}
			if strings.TrimSpace(problem.Error) == "" {
				t.Errorf("error body %q says nothing", recorder.Body.String())
			}
		})
	}
}
