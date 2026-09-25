package server

import (
	"log/slog"
	"net/http"

	"github.com/lukas-andrade/go-webservice/internal/handler"
	"github.com/lukas-andrade/go-webservice/internal/telemetry"
)

// PublicHandler serves the echo on every path. Health and metrics live on a
// separate admin port, otherwise paths like /metrics would stop being echoed.
// Tracing wraps everything so the log line can carry the trace id.
func PublicHandler(echo http.Handler, metrics *telemetry.Metrics, logger *slog.Logger) http.Handler {
	return telemetry.TraceMiddleware(Logging(logger, metrics.Middleware(echo)))
}

func AdminHandler(health *handler.Health, metrics *telemetry.Metrics) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health.Live)
	mux.HandleFunc("GET /readyz", health.Ready)
	mux.Handle("GET /metrics", metrics.Handler())
	return mux
}
