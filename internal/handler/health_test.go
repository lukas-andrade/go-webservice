package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthLive(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHealth().Live(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestHealthReadyFollowsState(t *testing.T) {
	h := NewHealth()
	check := func(want int) {
		t.Helper()
		rec := httptest.NewRecorder()
		h.Ready(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rec.Code != want {
			t.Fatalf("status = %d, want %d", rec.Code, want)
		}
	}

	check(http.StatusServiceUnavailable)
	h.SetReady(true)
	check(http.StatusOK)
	h.SetReady(false)
	check(http.StatusServiceUnavailable)
}
