package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/lukas-andrade/go-webservice/internal/echo"
)

type Echoer interface {
	Echo(req echo.Request) echo.Response
}

type Echo struct {
	svc          Echoer
	maxBodyBytes int64
	logger       *slog.Logger
}

func NewEcho(svc Echoer, maxBodyBytes int64, logger *slog.Logger) *Echo {
	return &Echo{svc: svc, maxBodyBytes: maxBodyBytes, logger: logger}
}

func (h *Echo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.maxBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("request body is larger than %d bytes", tooLarge.Limit))
			return
		}
		h.logger.WarnContext(r.Context(), "reading request body", "err", err)
		writeError(w, http.StatusBadRequest, "could not read request body")
		return
	}

	resp := h.svc.Echo(echo.Request{
		Path:    r.URL.Path,
		Headers: headersWithHost(r),
		Params:  r.URL.Query(),
		Body:    body,
	})
	writeJSON(w, http.StatusOK, resp)
}

// net/http moves Host out of the header map; put it back so the echo shows
// what the client actually sent.
func headersWithHost(r *http.Request) map[string][]string {
	h := r.Header.Clone()
	if h == nil {
		h = http.Header{}
	}
	if r.Host != "" {
		h.Set("Host", r.Host)
	}
	return h
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
