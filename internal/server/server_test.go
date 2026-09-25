package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServerStartAndShutdown(t *testing.T) {
	s := New("127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "hi")
	}))
	errs := make(chan error, 1)
	if err := s.Start(errs); err != nil {
		t.Fatalf("start: %v", err)
	}

	resp, err := http.Get("http://" + s.Addr())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "hi" {
		t.Errorf("body = %q", body)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	select {
	case err := <-errs:
		t.Errorf("a clean shutdown should not report an error, got %v", err)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestServerStartFailsOnBusyPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	s := New(ln.Addr().String(), http.NotFoundHandler())
	if err := s.Start(make(chan error, 1)); err == nil {
		t.Fatal("expected an error when the port is already taken")
	}
}
