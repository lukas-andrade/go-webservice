package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lukas-andrade/go-webservice/internal/echo"
	"github.com/lukas-andrade/go-webservice/internal/handler"
	"github.com/lukas-andrade/go-webservice/internal/telemetry"
)

func TestPublicHandlerEchoesReservedLookingPaths(t *testing.T) {
	metrics := telemetry.NewMetrics()
	h := PublicHandler(handler.NewEcho(echo.NewService(), 1024, discardLogger()), metrics, discardLogger())

	for _, path := range []string{"/metrics", "/healthz", "/anything/at/all"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"path":"`+path+`"`) {
			t.Errorf("%s: status %d body %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestAdminHandlerRoutes(t *testing.T) {
	metrics := telemetry.NewMetrics()
	health := handler.NewHealth()
	health.SetReady(true)
	h := AdminHandler(health, metrics)

	tests := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodGet, "/readyz", http.StatusOK},
		{http.MethodGet, "/metrics", http.StatusOK},
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
		{http.MethodGet, "/nope", http.StatusNotFound},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
		if rec.Code != tt.want {
			t.Errorf("%s %s = %d, want %d", tt.method, tt.path, rec.Code, tt.want)
		}
	}
}
