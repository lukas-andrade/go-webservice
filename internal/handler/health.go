package handler

import (
	"net/http"
	"sync/atomic"
)

// Health backs the liveness and readiness probes. Readiness is flipped off
// at the start of shutdown so load balancers stop sending traffic before
// in-flight requests are drained.
type Health struct {
	ready atomic.Bool
}

func NewHealth() *Health {
	return &Health{}
}

func (h *Health) SetReady(ready bool) {
	h.ready.Store(ready)
}

func (h *Health) Live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Health) Ready(w http.ResponseWriter, _ *http.Request) {
	if !h.ready.Load() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
