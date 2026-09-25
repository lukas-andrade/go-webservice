//go:build integration

// These tests talk to a running echo-service over the network. They're meant
// to run against the Docker image through `make test-integration`, which sets
// ECHO_BASE_URL and ECHO_ADMIN_URL.
package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

var (
	baseURL  = os.Getenv("ECHO_BASE_URL")
	adminURL = os.Getenv("ECHO_ADMIN_URL")
	client   = &http.Client{Timeout: 5 * time.Second}
)

type echoResponse struct {
	Headers map[string]string   `json:"Headers"`
	Params  map[string][]string `json:"Params"`
	Body    json.RawMessage     `json:"Body"`
	Path    string              `json:"Path"`
}

func TestMain(m *testing.M) {
	if baseURL == "" || adminURL == "" {
		fmt.Fprintln(os.Stderr, "ECHO_BASE_URL and ECHO_ADMIN_URL must be set; run `make test-integration`")
		os.Exit(1)
	}
	if err := waitUntilReady(30 * time.Second); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func waitUntilReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(adminURL + "/readyz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("echo-service at %s was not ready after %s", adminURL, timeout)
}

func doEcho(t *testing.T, method, path string, body io.Reader, headers map[string]string) (int, echoResponse) {
	t.Helper()
	req, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	var got echoResponse
	if resp.StatusCode == http.StatusOK {
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q", ct)
		}
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return resp.StatusCode, got
}

func TestEchoGetWithQueryAndHeaders(t *testing.T) {
	status, got := doEcho(t, http.MethodGet, "/v1/products/7?color=red&color=blue&page=2", nil,
		map[string]string{"X-Request-Id": "it-123"})

	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if got.Path != "/v1/products/7" {
		t.Errorf("path = %q", got.Path)
	}
	if c := got.Params["color"]; len(c) != 2 || c[0] != "red" || c[1] != "blue" {
		t.Errorf("params = %v", got.Params)
	}
	if got.Headers["X-Request-Id"] != "it-123" {
		t.Errorf("headers = %v", got.Headers)
	}
}

func TestEchoJSONBody(t *testing.T) {
	status, got := doEcho(t, http.MethodPost, "/orders", strings.NewReader(`{"sku":"A1","qty":3}`),
		map[string]string{"Content-Type": "application/json"})

	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if string(got.Body) != `{"sku":"A1","qty":3}` {
		t.Errorf("body = %s", got.Body)
	}
}

func TestEchoTextBody(t *testing.T) {
	status, got := doEcho(t, http.MethodPut, "/notes", strings.NewReader("just text"),
		map[string]string{"Content-Type": "text/plain"})

	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if string(got.Body) != `"just text"` {
		t.Errorf("body = %s", got.Body)
	}
}

func TestEchoAnyMethod(t *testing.T) {
	for _, method := range []string{http.MethodDelete, http.MethodPatch, http.MethodOptions} {
		if status, _ := doEcho(t, method, "/any", nil, nil); status != http.StatusOK {
			t.Errorf("%s status = %d", method, status)
		}
	}
}

func TestEchoRejectsOversizedBody(t *testing.T) {
	big := bytes.Repeat([]byte("a"), 1<<20+1)
	if status, _ := doEcho(t, http.MethodPost, "/upload", bytes.NewReader(big), nil); status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", status)
	}
}

func TestAdminEndpoints(t *testing.T) {
	for _, path := range []string{"/healthz", "/readyz"} {
		resp, err := client.Get(adminURL + path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s status = %d", path, resp.StatusCode)
		}
	}
}

func TestMetricsReflectTraffic(t *testing.T) {
	doEcho(t, http.MethodGet, "/counted", nil, nil)

	resp, err := client.Get(adminURL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if !strings.Contains(string(body), `http_requests_total{code="200",method="get"}`) {
		t.Errorf("expected a request counter for GET 200 in:\n%s", body)
	}
}
