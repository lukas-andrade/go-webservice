package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port            int
	AdminPort       int
	MaxBodyBytes    int64
	ShutdownTimeout time.Duration
	ServiceName     string
	LogLevel        string
	TracingEnabled  bool
}

func Default() Config {
	return Config{
		Port:            8080,
		AdminPort:       9090,
		MaxBodyBytes:    1 << 20,
		ShutdownTimeout: 15 * time.Second,
		ServiceName:     "echo-service",
		LogLevel:        "info",
	}
}

// Load reads the environment on top of the defaults. lookup is os.LookupEnv
// in production and a map in tests.
func Load(lookup func(string) (string, bool)) (Config, error) {
	cfg := Default()
	var err error

	if cfg.Port, err = intVar(lookup, "PORT", cfg.Port); err != nil {
		return Config{}, err
	}
	if cfg.AdminPort, err = intVar(lookup, "ADMIN_PORT", cfg.AdminPort); err != nil {
		return Config{}, err
	}
	maxBody, err := intVar(lookup, "MAX_BODY_BYTES", int(cfg.MaxBodyBytes))
	if err != nil {
		return Config{}, err
	}
	cfg.MaxBodyBytes = int64(maxBody)

	if v, ok := lookup("SHUTDOWN_TIMEOUT"); ok {
		if cfg.ShutdownTimeout, err = time.ParseDuration(v); err != nil {
			return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT: %w", err)
		}
	}
	if v, ok := lookup("OTEL_SERVICE_NAME"); ok && v != "" {
		cfg.ServiceName = v
	}
	if v, ok := lookup("LOG_LEVEL"); ok && v != "" {
		cfg.LogLevel = v
	}
	for _, name := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"} {
		if v, ok := lookup(name); ok && v != "" {
			cfg.TracingEnabled = true
		}
	}

	return cfg, cfg.validate()
}

func FromEnv() (Config, error) {
	return Load(os.LookupEnv)
}

func (c Config) validate() error {
	for name, port := range map[string]int{"PORT": c.Port, "ADMIN_PORT": c.AdminPort} {
		if port < 1 || port > 65535 {
			return fmt.Errorf("%s must be between 1 and 65535, got %d", name, port)
		}
	}
	if c.Port == c.AdminPort {
		return fmt.Errorf("PORT and ADMIN_PORT must differ, both are %d", c.Port)
	}
	if c.MaxBodyBytes <= 0 {
		return fmt.Errorf("MAX_BODY_BYTES must be positive, got %d", c.MaxBodyBytes)
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("SHUTDOWN_TIMEOUT must be positive, got %s", c.ShutdownTimeout)
	}
	return nil
}

func intVar(lookup func(string) (string, bool), name string, fallback int) (int, error) {
	v, ok := lookup(name)
	if !ok || v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return n, nil
}
