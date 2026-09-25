package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/lukas-andrade/go-webservice/internal/config"
	"github.com/lukas-andrade/go-webservice/internal/echo"
	"github.com/lukas-andrade/go-webservice/internal/handler"
	"github.com/lukas-andrade/go-webservice/internal/server"
	"github.com/lukas-andrade/go-webservice/internal/telemetry"
)

func main() {
	if err := run(); err != nil {
		slog.Error("echo-service stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := telemetry.SetupTracing(ctx, cfg.ServiceName, cfg.TracingEnabled)
	if err != nil {
		return err
	}

	metrics := telemetry.NewMetrics()
	health := handler.NewHealth()
	echoHandler := handler.NewEcho(echo.NewService(), cfg.MaxBodyBytes, logger)

	public := server.New(fmt.Sprintf(":%d", cfg.Port), server.PublicHandler(echoHandler, metrics, logger))
	admin := server.New(fmt.Sprintf(":%d", cfg.AdminPort), server.AdminHandler(health, metrics))

	errs := make(chan error, 2)
	for _, s := range []*server.Server{public, admin} {
		if err := s.Start(errs); err != nil {
			return err
		}
	}
	health.SetReady(true)
	logger.Info("echo-service started",
		"addr", public.Addr(), "admin_addr", admin.Addr(), "tracing", cfg.TracingEnabled)

	var serveErr error
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case serveErr = <-errs:
		logger.Error("server failed, shutting down", "err", serveErr)
	}
	health.SetReady(false)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return errors.Join(
		serveErr,
		public.Shutdown(shutdownCtx),
		admin.Shutdown(shutdownCtx),
		shutdownTracing(shutdownCtx),
	)
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
