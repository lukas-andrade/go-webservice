package telemetry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsMiddlewareCountsRequests(t *testing.T) {
	m := NewMetrics()
	h := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	for _, path := range []string{"/a", "/b", "/missing"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	if got := testutil.ToFloat64(m.requests.WithLabelValues("get", "200")); got != 2 {
		t.Errorf("200s = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues("get", "404")); got != 1 {
		t.Errorf("404s = %v, want 1", got)
	}
}

func TestMetricsHandlerExposesRegistry(t *testing.T) {
	m := NewMetrics()
	m.Middleware(http.NotFoundHandler()).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	for _, want := range []string{"http_requests_total", "http_request_duration_seconds", "go_goroutines"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("metrics output is missing %s", want)
		}
	}
}
