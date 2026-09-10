package rest

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func nearSeconds(got int, want int, tolerance int) bool {
	diff := got - want
	if diff < 0 {
		diff = -diff
	}

	return diff <= tolerance
}

// expectValidationError asserts a 400 whose details name field with message.
func expectValidationError(t *testing.T, handler *http.ServeMux, path string, field string, message string) {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, path, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("%s: status = %d, want %d", path, recorder.Code, http.StatusBadRequest)
	}

	if got := decodeError(t, recorder.Body).Details[field]; got != message {
		t.Errorf("%s: details[%s] = %q, want %q", path, field, got, message)
	}
}

func TestAPISplitsEveryKilometer(t *testing.T) {
	handler := newTestHandlerWithMode(t, false)

	var got splitsResponse
	if code := getJSON(t, handler, &got, "/api/v1/splits?dist=10000&pace=5m0s&step=1000"); code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}

	if len(got.Splits) != 10 {
		t.Fatalf("len(splits) = %d, want 10", len(got.Splits))
	}

	want := splitJSON{Distance: 10000, Elapsed: durationValue{Text: "50m0s", Seconds: 3000}}
	if last := got.Splits[len(got.Splits)-1]; last != want {
		t.Errorf("last split = %+v, want %+v", last, want)
	}
}

func TestAPISplitsDefaultToEveryKilometer(t *testing.T) {
	handler := newTestHandlerWithMode(t, false)

	var got splitsResponse
	getJSON(t, handler, &got, "/api/v1/splits?dist=5000&pace=4m0s")

	if len(got.Splits) != 5 {
		t.Errorf("len(splits) = %d, want 5", len(got.Splits))
	}
}

func TestAPISplitsRejectBadInput(t *testing.T) {
	handler := newTestHandlerWithMode(t, false)

	tests := []struct {
		name    string
		path    string
		field   string
		message string
	}{
		{name: "missing distance", path: "/api/v1/splits?pace=5m0s", field: "dist", message: "field is required"},
		{name: "zero pace", path: "/api/v1/splits?dist=5000&pace=0s", field: "pace", message: "value should be greater than zero"},
		{name: "zero step", path: "/api/v1/splits?dist=5000&pace=5m0s&step=0", field: "step", message: "value should be greater than zero"},
		{name: "non numeric step", path: "/api/v1/splits?dist=5000&pace=5m0s&step=abc", field: "step", message: "incorrect type should be number"},
		// 100 km in 10 m steps would put 10 000 rows in a single response.
		{name: "too many splits", path: "/api/v1/splits?dist=100000&pace=5m0s&step=10", field: "step", message: "too many splits, use a larger step"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectValidationError(t, handler, tt.path, tt.field, tt.message)
		})
	}
}

func TestAPIPredictsStandardDistances(t *testing.T) {
	handler := newTestHandlerWithMode(t, false)

	var got predictResponse
	if code := getJSON(t, handler, &got, "/api/v1/predict?dist=5000&time=20m0s"); code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}

	wantDistances := []int{1000, 3000, 5000, 10000, 21097, 42195}
	if len(got.Predictions) != len(wantDistances) {
		t.Fatalf("len(predictions) = %d, want %d", len(got.Predictions), len(wantDistances))
	}

	for i, want := range wantDistances {
		if got.Predictions[i].Distance != want {
			t.Errorf("prediction %d distance = %d, want %d", i, got.Predictions[i].Distance, want)
		}
	}

	for _, prediction := range got.Predictions {
		switch prediction.Distance {
		case 5000:
			if prediction.Riegel.Seconds != 1200 || prediction.Cameron.Seconds != 1200 {
				t.Errorf("known distance predicted as %+v, want 1200 s from both models", prediction)
			}
		case 10000:
			// 20 * 2^1.06 = 41.699 min and Cameron's 41.661 min, both by hand.
			if !nearSeconds(prediction.Riegel.Seconds, 2502, 2) {
				t.Errorf("riegel 10 km = %d s, want 2502 ±2", prediction.Riegel.Seconds)
			}
			if !nearSeconds(prediction.Cameron.Seconds, 2500, 2) {
				t.Errorf("cameron 10 km = %d s, want 2500 ±2", prediction.Cameron.Seconds)
			}
		}
	}
}

func TestAPIPredictRejectsBadInput(t *testing.T) {
	handler := newTestHandlerWithMode(t, false)

	expectValidationError(t, handler, "/api/v1/predict?dist=5000", "time", "field is required")
	expectValidationError(t, handler, "/api/v1/predict?dist=5000&time=0s", "time", "value should be greater than zero")
	expectValidationError(t, handler, "/api/v1/predict?time=20m0s", "dist", "field is required")
}

func TestAPIVDOTFromRaceResult(t *testing.T) {
	handler := newTestHandlerWithMode(t, false)

	var got vdotResponse
	if code := getJSON(t, handler, &got, "/api/v1/vdot?dist=5000&time=19m57s"); code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}

	// 19:57 over 5 km is VDOT 50 in Daniels' published table.
	if math.Abs(got.VDOT-50) > 0.3 {
		t.Errorf("vdot = %.2f, want 50 ±0.3", got.VDOT)
	}

	// Published VDOT 50 paces per kilometer: threshold 4'15, marathon 4'31.
	if !nearSeconds(got.Paces.Threshold.Seconds, 255, 3) {
		t.Errorf("threshold pace = %d s/km, want 255 ±3", got.Paces.Threshold.Seconds)
	}
	if !nearSeconds(got.Paces.Marathon.Seconds, 271, 3) {
		t.Errorf("marathon pace = %d s/km, want 271 ±3", got.Paces.Marathon.Seconds)
	}

	var marathon *equivalentJSON
	for i := range got.Equivalents {
		if got.Equivalents[i].Distance == 42195 {
			marathon = &got.Equivalents[i]
		}
	}
	if marathon == nil {
		t.Fatal("equivalents do not include the marathon")
	}

	// Published VDOT 50 marathon: 3:10:49.
	if !nearSeconds(marathon.Time.Seconds, 11449, 15) {
		t.Errorf("equivalent marathon = %d s, want 11449 ±15", marathon.Time.Seconds)
	}
}

func TestAPIVDOTRejectsBadInput(t *testing.T) {
	handler := newTestHandlerWithMode(t, false)

	expectValidationError(t, handler, "/api/v1/vdot?time=20m0s", "dist", "field is required")
	expectValidationError(t, handler, "/api/v1/vdot?dist=5000&time=0s", "time", "value should be greater than zero")
}
