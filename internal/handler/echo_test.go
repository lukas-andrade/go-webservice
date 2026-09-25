package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/lukas-andrade/go-webservice/internal/echo"
)

type echoResponse struct {
	Headers map[string][]string `json:"headers"`
	Params  map[string][]string `json:"params"`
	Body    json.RawMessage     `json:"body"`
	Path    string              `json:"path"`
}

type spyEchoer struct {
	got echo.Request
}

func (s *spyEchoer) Echo(req echo.Request) echo.Response {
	s.got = req
	return echo.Response{Path: "from-spy"}
}

func newTestHandler(maxBody int64) *Echo {
	return NewEcho(echo.NewService(), maxBody, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestEchoMapsRequestForService(t *testing.T) {
	spy := &spyEchoer{}
	h := NewEcho(spy, 1024, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodPut, "http://echo.local/a/b?x=1&x=2", strings.NewReader("payload"))
	req.Header.Set("X-Trace", "abc")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if spy.got.Path != "/a/b" {
		t.Errorf("path = %q", spy.got.Path)
	}
	if got := spy.got.Params["x"]; len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Errorf("params = %v", spy.got.Params)
	}
	if got := spy.got.Headers["X-Trace"]; len(got) != 1 || got[0] != "abc" {
		t.Errorf("headers = %v", spy.got.Headers)
	}
	if got := spy.got.Headers["Host"]; len(got) != 1 || got[0] != "echo.local" {
		t.Errorf("host header = %v", got)
	}
	if string(spy.got.Body) != "payload" {
		t.Errorf("body = %q", spy.got.Body)
	}
	if _, ok := req.Header["Host"]; ok {
		t.Error("handler must not mutate the incoming request headers")
	}
}

func TestEchoReturnsJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users?active=true", strings.NewReader(`{"name":"ana"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newTestHandler(1024).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}
	var got echoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Path != "/users" || got.Params["active"][0] != "true" || string(got.Body) != `{"name":"ana"}` {
		t.Errorf("unexpected echo: %+v", got)
	}
}

func TestEchoRejectsOversizedBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", 11)))
	rec := httptest.NewRecorder()

	newTestHandler(10).ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "larger than 10 bytes") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestEchoHandlesUnreadableBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", failingReader{})
	rec := httptest.NewRecorder()

	newTestHandler(1024).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// Run with -race: one handler instance serving many goroutines at once.
func TestEchoIsSafeForConcurrentUse(t *testing.T) {
	h := newTestHandler(1024)
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			path := "/c/" + string(rune('a'+i%26))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(path)))

			var got echoResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Errorf("decode: %v", err)
				return
			}
			if got.Path != path || string(got.Body) != `"`+path+`"` {
				t.Errorf("request %s got someone else's echo: %+v", path, got)
			}
		})
	}
	wg.Wait()
}
