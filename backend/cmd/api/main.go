package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/app"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/logx"
	"github.com/tokpath/tokstudio/backend/internal/platform/otelx"
	"github.com/tokpath/tokstudio/backend/internal/platform/redisx"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	logger := logx.New(cfg.LogLevel, os.Stdout)
	logger.Info().Fields(cfg.RedactedMap()).Msg("api_starting")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTrace, err := otelx.Setup(ctx, cfg.OTELServiceName, cfg.OTELEndpoint)
	if err != nil {
		logger.Fatal().Err(err).Msg("otel_setup_failed")
	}
	defer func() { _ = shutdownTrace(context.Background()) }()

	gdb, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("postgres_open_failed")
	}
	rdb, err := redisx.Open(cfg.RedisURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("redis_open_failed")
	}

	application := app.New(cfg, gdb, rdb, logger)
	defer application.Close()
	if err := application.Migrate(); err != nil {
		logger.Fatal().Err(err).Msg("migrate_failed")
	}
	if err := application.Bootstrap(ctx); err != nil {
		logger.Fatal().Err(err).Msg("bootstrap_failed")
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           application.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		logger.Info().Str("addr", cfg.HTTPAddr).Msg("api_listen")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("api_listen_failed")
		}
	}()
	go application.Media.Run(ctx)

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	logger.Info().Msg("api_stopped")
}
