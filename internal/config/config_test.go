package config

import (
	"strings"
	"testing"
	"time"
)

func lookupFrom(env map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := env[k]
		return v, ok
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(lookupFrom(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != Default() {
		t.Errorf("got %+v, want defaults %+v", cfg, Default())
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := Load(lookupFrom(map[string]string{
		"PORT":                        "3000",
		"ADMIN_PORT":                  "3001",
		"MAX_BODY_BYTES":              "512",
		"SHUTDOWN_TIMEOUT":            "2s",
		"OTEL_SERVICE_NAME":           "echo-test",
		"LOG_LEVEL":                   "debug",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4318",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Config{
		Port:            3000,
		AdminPort:       3001,
		MaxBodyBytes:    512,
		ShutdownTimeout: 2 * time.Second,
		ServiceName:     "echo-test",
		LogLevel:        "debug",
		TracingEnabled:  true,
	}
	if cfg != want {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"non numeric port", map[string]string{"PORT": "http"}, "PORT"},
		{"port out of range", map[string]string{"PORT": "70000"}, "PORT must be between"},
		{"same ports", map[string]string{"PORT": "9090"}, "must differ"},
		{"zero body limit", map[string]string{"MAX_BODY_BYTES": "0"}, "MAX_BODY_BYTES"},
		{"bad duration", map[string]string{"SHUTDOWN_TIMEOUT": "soon"}, "SHUTDOWN_TIMEOUT"},
		{"negative duration", map[string]string{"SHUTDOWN_TIMEOUT": "-1s"}, "SHUTDOWN_TIMEOUT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(lookupFrom(tt.env))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("got error %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}
