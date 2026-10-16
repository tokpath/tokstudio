package main

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/logx"
	"github.com/tokpath/tokstudio/backend/internal/platform/otelx"
	"github.com/tokpath/tokstudio/backend/internal/portal"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--healthcheck" {
		if portal.Healthcheck(os.Getenv("TOKENHUB_WORKER_HTTP_ADDR"), "8081") {
			return
		}
		os.Exit(1)
	}
	cfg, err := portal.LoadConfig()
	if err != nil {
		panic(err)
	}
	logger := logx.New(cfg.LogLevel, os.Stdout)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	telemetryShutdown, err := otelx.Setup(ctx, "tokenhub-worker", cfg.OTELEndpoint)
	if err != nil {
		logger.Fatal().Err(err).Msg("telemetry_configuration_failed")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = telemetryShutdown(cleanup)
	}()
	gdb, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("postgres_open_failed")
	}
	application, err := portal.New(ctx, cfg, gdb)
	if err != nil {
		logger.Fatal().Err(err).Msg("worker_not_ready")
	}
	defer application.Close()
	addr := os.Getenv("TOKENHUB_WORKER_HTTP_ADDR")
	if addr == "" {
		addr = ":8081"
	}
	server := &http.Server{Addr: addr, Handler: application.InternalRouter(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go func() {
		if e := server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			logger.Error().Msg("worker_health_listener_failed")
			stop()
		}
	}()
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(cleanup)
	}()
	concurrency, _ := strconv.Atoi(os.Getenv("TOKENHUB_WORKER_CONCURRENCY"))
	logger.Info().Msg("worker_ready")
	application.RunWorker(ctx, concurrency, func(kind string, _ error) {
		logger.Error().Str("task", kind).Msg("worker_task_requires_retry_or_review")
	})
}
