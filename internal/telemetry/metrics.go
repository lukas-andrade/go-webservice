package telemetry

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics owns its own registry instead of the global one, so tests can
// build as many instances as they like without duplicate-registration panics.
//
// Labels are limited to method and status code on purpose: the echo accepts
// any path, and labelling by path would let clients blow up cardinality.
type Metrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func NewMetrics() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "HTTP requests handled, by method and status code.",
		}, []string{"method", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Time spent serving HTTP requests.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "code"}),
	}
	m.registry.MustRegister(
		m.requests, m.duration,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return m
}

func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return promhttp.InstrumentHandlerDuration(m.duration,
		promhttp.InstrumentHandlerCounter(m.requests, next))
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{Registry: m.registry})
}
