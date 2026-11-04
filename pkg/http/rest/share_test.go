package rest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"gorun/pkg/plan"
)

type preparingStub struct {
	calls    int
	user     int64
	p        plan.Plan
	language string
	err      error
}

func (s *preparingStub) PrepareInlineMessage(ctx context.Context, user int64, p plan.Plan, language string) (*models.PreparedInlineMessage, error) {
	s.calls++
	s.user = user
	s.p = p
	s.language = language
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("missing deadline")
	}
	return &models.PreparedInlineMessage{ID: "prepared-1", ExpirationDate: 1900000000}, s.err
}

func TestShareAuthenticatesAndValidatesBeforePreparing(t *testing.T) {
	fresh := "tma " + signInitData("token", time.Now(), `{"id":42,"first_name":"Ann"}`)
	for _, tc := range []struct {
		name, auth, body string
		status           int
	}{
		{"valid", fresh, `{"distance":42195,"time_seconds":12600,"language":"ru"}`, 200},
		{"missing auth", "", `{}`, 401},
		{"forged", "tma " + signInitData("other", time.Now(), `{"id":99}`), `{}`, 401},
		{"expired", "tma " + signInitData("token", time.Now().Add(-48*time.Hour), `{"id":42}`), `{}`, 401},
		{"missing user", "tma " + signInitData("token", time.Now(), `{}`), `{}`, 401},
		{"oversized", fresh, strings.Repeat(" ", 1025), 400},
		{"zero", fresh, `{"distance":0,"time_seconds":12600,"language":"ru"}`, 400},
		{"too far", fresh, `{"distance":1000001,"time_seconds":12600,"language":"ru"}`, 400},
		{"too long", fresh, `{"distance":42195,"time_seconds":3600001,"language":"ru"}`, 400},
		{"negative", fresh, `{"distance":42195,"time_seconds":-1,"language":"en"}`, 400},
		{"language", fresh, `{"distance":42195,"time_seconds":12600,"language":"<script>"}`, 400},
		{"fraction", fresh, `{"distance":42195.5,"time_seconds":12600,"language":"en"}`, 400},
		{"client user", fresh, `{"distance":42195,"time_seconds":12600,"language":"ru","user_id":99}`, 400},
		{"trailing", fresh, `{"distance":42195,"time_seconds":12600,"language":"ru"}{}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &preparingStub{}
			r := httptest.NewRequest("POST", "/api/v1/share", strings.NewReader(tc.body))
			r.Header.Set("Authorization", tc.auth)
			w := httptest.NewRecorder()
			prepareShareHandler("token", stub).ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "private, no-store" {
				t.Error("share response can be cached")
			}
			if tc.status == 200 {
				if stub.calls != 1 || stub.user != 42 || stub.p.Distance != 42195 || stub.p.Time != 210*time.Minute || stub.language != "ru" || !strings.Contains(w.Body.String(), `"id":"prepared-1"`) {
					t.Fatalf("%+v %s", stub, w.Body.String())
				}
			} else if stub.calls != 0 {
				t.Fatal("invalid request reached Telegram")
			}
		})
	}
}

func TestShareReturnsGatewayErrorWithoutLeakingTelegramError(t *testing.T) {
	stub := &preparingStub{err: errors.New("secret transport detail")}
	r := httptest.NewRequest("POST", "/api/v1/share", strings.NewReader(`{"distance":10000,"time_seconds":3000,"language":"en"}`))
	r.Header.Set("Authorization", "tma "+signInitData("token", time.Now(), `{"id":42}`))
	w := httptest.NewRecorder()
	prepareShareHandler("token", stub).ServeHTTP(w, r)
	if w.Code != http.StatusBadGateway || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestShareRouteExistsWithoutHistoryOrTelegram(t *testing.T) {
	for _, debug := range []bool{true, false} {
		h := newTestHandlerWithMode(t, debug)
		r := httptest.NewRequest("POST", "/api/v1/share", strings.NewReader(`{"distance":10000,"time_seconds":3000,"language":"en"}`))
		r.Header.Set("Authorization", "tma "+signInitData("token", time.Now(), `{"id":42}`))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 503 {
			t.Fatalf("debug=%v status=%d", debug, w.Code)
		}
	}
}
