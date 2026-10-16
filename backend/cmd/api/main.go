package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/logx"
	"github.com/tokpath/tokstudio/backend/internal/platform/otelx"
	"github.com/tokpath/tokstudio/backend/internal/portal"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--healthcheck" {
		if portal.Healthcheck(os.Getenv("TOKENHUB_HTTP_ADDR"), "8080") {
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
	telemetryShutdown, err := otelx.Setup(ctx, "tokenhub-api", cfg.OTELEndpoint)
	if err != nil {
		logger.Fatal().Err(err).Msg("telemetry_configuration_failed")
	}
	defer func() { _ = telemetryShutdown(context.Background()) }()
	gdb, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("postgres_open_failed")
	}
	application, err := portal.New(ctx, cfg, gdb)
	if err != nil {
		logger.Fatal().Err(err).Msg("application_not_ready")
	}
	defer application.Close()
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: application.Router(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1 << 20}
	go func() {
		logger.Info().Str("addr", cfg.HTTPAddr).Msg("api_listen")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("api_listen_failed")
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
}
