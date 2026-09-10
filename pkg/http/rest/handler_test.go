package rest

import (
	"bytes"
	"encoding/json"
	"gorun/pkg/calculator"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func newTestHandler(t *testing.T) *http.ServeMux {
	t.Helper()

	assets := fstest.MapFS{
		"assets/index.html": &fstest.MapFile{Data: []byte("<!doctype html><html></html>")},
	}

	handler, err := NewHandler(true, "token", nil, calculator.NewService(), assets)
	if err != nil {
		t.Fatalf("create test handler: %v", err)
	}

	return handler
}

func TestHealthz(t *testing.T) {
	handler := newTestHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", response.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("unable to parse response body: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("unexpected body status: %q", body["status"])
	}
}

func TestCalculateTimeValidationError(t *testing.T) {
	handler := newTestHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/time?dist=0&pace=bad", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", response.Code, http.StatusBadRequest)
	}

	var body errorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("unable to parse response body: %v", err)
	}

	if body.Error != "validation failed" {
		t.Fatalf("unexpected error message: %q", body.Error)
	}
	if body.Details["dist"] != "value should be greater than zero" {
		t.Fatalf("unexpected dist details: %q", body.Details["dist"])
	}
	if body.Details["pace"] == "" {
		t.Fatal("pace validation details are empty")
	}
}

func TestCalculatePaceSuccess(t *testing.T) {
	handler := newTestHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/pace?dist=5000&time=20m45s", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", response.Code, http.StatusOK)
	}

	if got := strings.TrimSpace(response.Body.String()); got != "4m9s" {
		t.Fatalf("unexpected pace result: got %q want %q", got, "4m9s")
	}
}

func decodeError(t *testing.T, body *bytes.Buffer) errorResponse {
	t.Helper()

	var resp errorResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}

	return resp
}

func TestCalculateTimeReportsMissingFieldsAsRequired(t *testing.T) {
	handler := newTestHandler(t)

	request := httptest.NewRequest(http.MethodGet, "/time?pace=4m50s", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}

	if got, want := decodeError(t, recorder.Body).Details["dist"], "field is required"; got != want {
		t.Errorf("details[dist] = %q, want %q", got, want)
	}
}

func TestCalculatePaceReportsMissingFieldsAsRequired(t *testing.T) {
	handler := newTestHandler(t)

	request := httptest.NewRequest(http.MethodGet, "/pace?time=1h38m48s", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}

	if got, want := decodeError(t, recorder.Body).Details["dist"], "field is required"; got != want {
		t.Errorf("details[dist] = %q, want %q", got, want)
	}
}

func TestCalculateTimeStillRejectsNonNumericDistance(t *testing.T) {
	handler := newTestHandler(t)

	request := httptest.NewRequest(http.MethodGet, "/time?pace=4m50s&dist=abc", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if got, want := decodeError(t, recorder.Body).Details["dist"], "incorrect type should be number"; got != want {
		t.Errorf("details[dist] = %q, want %q", got, want)
	}
}
