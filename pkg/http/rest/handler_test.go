package rest

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"gorun/pkg/calculator"
	"gorun/pkg/history"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func newTestHandler(t *testing.T) *http.ServeMux {
	t.Helper()

	return newTestHandlerWithMode(t, true)
}

func newTestHandlerWithMode(t *testing.T, debugMode bool) *http.ServeMux {
	t.Helper()

	return newTestHandlerWith(t, debugMode, nil)
}

// newTestHandlerWith builds the handler with the bot token "token"; a nil store
// leaves the history endpoints unconfigured.
func newTestHandlerWith(t *testing.T, debugMode bool, store *history.Store) *http.ServeMux {
	t.Helper()

	// крупнее MinSize компрессора, иначе сжатие не включится
	page := []byte("<!doctype html><html><body>" + strings.Repeat("pacer ", 1000) + "</body></html>")

	assets := fstest.MapFS{
		"assets/index.html": &fstest.MapFile{Data: page},
	}

	handler, err := NewHandler(debugMode, "token", nil, calculator.NewService(), store, assets)
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
	request := httptest.NewRequest(http.MethodGet, "/api/v1/time?dist=0&pace=bad", nil)
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
	request := httptest.NewRequest(http.MethodGet, "/api/v1/pace?dist=5000&time=20m45s", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", response.Code, http.StatusOK)
	}

	var body calcResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("unable to parse response body: %v", err)
	}

	if body.Result != "4m9s" {
		t.Fatalf("unexpected pace result: got %q want %q", body.Result, "4m9s")
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

	request := httptest.NewRequest(http.MethodGet, "/api/v1/time?pace=4m50s", nil)
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

	request := httptest.NewRequest(http.MethodGet, "/api/v1/pace?time=1h38m48s", nil)
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

	request := httptest.NewRequest(http.MethodGet, "/api/v1/time?pace=4m50s&dist=abc", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if got, want := decodeError(t, recorder.Body).Details["dist"], "incorrect type should be number"; got != want {
		t.Errorf("details[dist] = %q, want %q", got, want)
	}
}

func TestAssetsAreGzippedWhenClientAcceptsIt(t *testing.T) {
	handler := newTestHandler(t)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want %q", got, "gzip")
	}

	reader, err := gzip.NewReader(recorder.Body)
	if err != nil {
		t.Fatalf("open gzip reader: %v", err)
	}
	defer func() { _ = reader.Close() }()

	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read gzipped body: %v", err)
	}

	if !strings.Contains(string(body), "<!doctype html>") {
		t.Errorf("decompressed body does not look like the page: %.40q", body)
	}
}

func TestAssetsAreServedPlainWhenGzipNotAccepted(t *testing.T) {
	handler := newTestHandler(t)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Accept-Encoding", "identity")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}

	if !strings.Contains(recorder.Body.String(), "<!doctype html>") {
		t.Errorf("body does not look like the page: %.40q", recorder.Body.String())
	}
}

// getJSON issues a GET and decodes the JSON body into target.
func getJSON(t *testing.T, handler *http.ServeMux, target any, path string) int {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, path, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if target != nil {
		if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
			t.Fatalf("decode %s: %v (body %q)", path, err, recorder.Body.String())
		}
	}

	return recorder.Code
}

func TestAPICalculatesTimeInBothModes(t *testing.T) {
	for _, debugMode := range []bool{true, false} {
		handler := newTestHandlerWithMode(t, debugMode)

		var got calcResponse
		code := getJSON(t, handler, &got, "/api/v1/time?pace=5m0s&dist=10000")

		if code != http.StatusOK {
			t.Fatalf("debug=%v: status = %d, want 200", debugMode, code)
		}

		want := calcResponse{Result: "50m0s", TotalSeconds: 3000, Hours: 0, Minutes: 50, Seconds: 0}
		if got != want {
			t.Errorf("debug=%v: got %+v, want %+v", debugMode, got, want)
		}
	}
}

func TestAPICalculatesPaceInBothModes(t *testing.T) {
	for _, debugMode := range []bool{true, false} {
		handler := newTestHandlerWithMode(t, debugMode)

		var got calcResponse
		code := getJSON(t, handler, &got, "/api/v1/pace?dist=10000&time=50m0s")

		if code != http.StatusOK {
			t.Fatalf("debug=%v: status = %d, want 200", debugMode, code)
		}

		want := calcResponse{Result: "5m0s", TotalSeconds: 300, Hours: 0, Minutes: 5, Seconds: 0}
		if got != want {
			t.Errorf("debug=%v: got %+v, want %+v", debugMode, got, want)
		}
	}
}

func TestAPIReportsValidationErrors(t *testing.T) {
	handler := newTestHandlerWithMode(t, false)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/time?pace=5m0s", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}

	if got, want := decodeError(t, recorder.Body).Details["dist"], "field is required"; got != want {
		t.Errorf("details[dist] = %q, want %q", got, want)
	}
}

func TestAPISplitsHoursForLongRaces(t *testing.T) {
	handler := newTestHandlerWithMode(t, false)

	var got calcResponse
	getJSON(t, handler, &got, "/api/v1/time?pace=10m0s&dist=100000")

	// 100 км по 10:00/км = 1000 минут = 16 ч 40 мин
	want := calcResponse{Result: "16h40m0s", TotalSeconds: 60000, Hours: 16, Minutes: 40, Seconds: 0}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
