package server

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestLoggingRecordsStatus(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := Logging(logger, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/pot", nil))

	for _, want := range []string{`"status":418`, `"method":"DELETE"`, `"path":"/pot"`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("log line %s is missing %s", buf.String(), want)
		}
	}
}
