package rest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorun/pkg/history"
)

// runJSON is the saved-run contract as a client sees it, decoded independently
// of the production types.
type runJSON struct {
	ID       int64 `json:"id"`
	Distance int   `json:"distance"`
	Time     struct {
		Text    string `json:"text"`
		Seconds int    `json:"seconds"`
	} `json:"time"`
	SavedAt string `json:"saved_at"`
}

// signedIn is the Authorization header of a Mini App user of the test bot.
func signedIn(userID int64) string {
	user := `{"id":` + strconv.FormatInt(userID, 10) + `,"first_name":"Runner"}`

	return "tma " + signInitData("token", time.Now(), user)
}

func newHistoryHandler(t *testing.T) *http.ServeMux {
	t.Helper()

	store, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	return newTestHandlerWith(t, false, store)
}

func call(t *testing.T, handler *http.ServeMux, method string, path string, body string, authorization string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	request := httptest.NewRequest(method, path, reader)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	return recorder
}

func saveRun(t *testing.T, handler *http.ServeMux, userID int64, body string) runJSON {
	t.Helper()

	recorder := call(t, handler, http.MethodPost, "/api/v1/runs", body, signedIn(userID))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("save: status = %d, want %d (body %q)", recorder.Code, http.StatusCreated, recorder.Body.String())
	}

	var created struct {
		Run runJSON `json:"run"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode saved run: %v", err)
	}

	return created.Run
}

func listRuns(t *testing.T, handler *http.ServeMux, userID int64) []runJSON {
	t.Helper()

	recorder := call(t, handler, http.MethodGet, "/api/v1/runs", "", signedIn(userID))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list: status = %d, want %d (body %q)", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	// An empty history must be an empty array, not null, for the page.
	if !strings.Contains(recorder.Body.String(), `"runs":[`) {
		t.Errorf("list body %q has no runs array", recorder.Body.String())
	}

	var listed struct {
		Runs []runJSON `json:"runs"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode runs: %v", err)
	}

	return listed.Runs
}

func TestRunsRequireSignIn(t *testing.T) {
	handler := newHistoryHandler(t)

	for _, request := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/runs"},
		{method: http.MethodPost, path: "/api/v1/runs", body: `{"distance":10000,"time_seconds":3000}`},
		{method: http.MethodDelete, path: "/api/v1/runs/1"},
	} {
		recorder := call(t, handler, request.method, request.path, request.body, "")

		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without sign-in: status = %d, want %d",
				request.method, request.path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func TestSaveThenListRuns(t *testing.T) {
	handler := newHistoryHandler(t)

	created := saveRun(t, handler, 42, `{"distance":21097,"time_seconds":5928}`)
	if created.ID == 0 || created.Distance != 21097 || created.Time.Seconds != 5928 || created.Time.Text != "1h38m48s" {
		t.Errorf("created = %+v, want the half marathon in 1h38m48s with an id", created)
	}
	if _, err := time.Parse(time.RFC3339, created.SavedAt); err != nil {
		t.Errorf("saved_at %q is not RFC 3339: %v", created.SavedAt, err)
	}

	runs := listRuns(t, handler, 42)
	if len(runs) != 1 || runs[0].ID != created.ID {
		t.Errorf("runs = %+v, want exactly the saved run %d", runs, created.ID)
	}
}

func TestRunsAreSeparatedBetweenUsers(t *testing.T) {
	handler := newHistoryHandler(t)

	saveRun(t, handler, 42, `{"distance":10000,"time_seconds":3000}`)

	if runs := listRuns(t, handler, 7); len(runs) != 0 {
		t.Errorf("another user sees %d runs, want 0", len(runs))
	}
}

func TestDeleteRun(t *testing.T) {
	handler := newHistoryHandler(t)
	created := saveRun(t, handler, 42, `{"distance":10000,"time_seconds":3000}`)
	path := "/api/v1/runs/" + strconv.FormatInt(created.ID, 10)

	recorder := call(t, handler, http.MethodDelete, path, "", signedIn(7))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("delete by another user: status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if got := decodeError(t, recorder.Body).Error; got != "run not found" {
		t.Errorf("error = %q, want %q", got, "run not found")
	}

	recorder = call(t, handler, http.MethodDelete, path, "", signedIn(42))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("delete by owner: status = %d, want %d", recorder.Code, http.StatusNoContent)
	}

	if runs := listRuns(t, handler, 42); len(runs) != 0 {
		t.Errorf("run still listed after deletion: %+v", runs)
	}
}

func TestSaveRunRejectsBadInput(t *testing.T) {
	handler := newHistoryHandler(t)

	recorder := call(t, handler, http.MethodPost, "/api/v1/runs", "not json", signedIn(42))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("garbage body: status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if got := decodeError(t, recorder.Body).Error; got != "invalid request body" {
		t.Errorf("garbage body: error = %q, want %q", got, "invalid request body")
	}

	for field, body := range map[string]string{
		"distance":     `{"distance":0,"time_seconds":5928}`,
		"time_seconds": `{"distance":10000,"time_seconds":0}`,
	} {
		recorder := call(t, handler, http.MethodPost, "/api/v1/runs", body, signedIn(42))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want %d", field, recorder.Code, http.StatusBadRequest)
			continue
		}
		if got := decodeError(t, recorder.Body).Details[field]; got != "value should be greater than zero" {
			t.Errorf("%s: details = %q, want %q", field, got, "value should be greater than zero")
		}
	}
}

func TestDeleteRejectsNonNumericID(t *testing.T) {
	handler := newHistoryHandler(t)

	recorder := call(t, handler, http.MethodDelete, "/api/v1/runs/abc", "", signedIn(42))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if got := decodeError(t, recorder.Body).Error; got != "invalid run id" {
		t.Errorf("error = %q, want %q", got, "invalid run id")
	}
}

func TestRunsWithoutStorageConfigured(t *testing.T) {
	handler := newTestHandlerWithMode(t, false)

	recorder := call(t, handler, http.MethodGet, "/api/v1/runs", "", signedIn(42))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if got := decodeError(t, recorder.Body).Error; got != "history is not configured" {
		t.Errorf("error = %q, want %q", got, "history is not configured")
	}
}
