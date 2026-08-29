package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/tokpath/tokstudio/backend/internal/app"
	"github.com/tokpath/tokstudio/backend/internal/outbox"
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
	logger := logx.New(cfg.LogLevel, os.Stdout).With().Str("component", "outbox-worker").Logger()
	logger.Info().Fields(cfg.RedactedMap()).Msg("worker_starting")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTrace, err := otelx.Setup(ctx, cfg.OTELServiceName+"-worker", cfg.OTELEndpoint)
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
	if err := application.Migrate(); err != nil {
		logger.Fatal().Err(err).Msg("migrate_failed")
	}

	worker := outbox.NewWorker(gdb, rdb, logger)
	go worker.Run(ctx)
	go application.Billing.RunReaper(ctx)
	go application.Media.Run(ctx)
	go application.Plans.RunRenewal(ctx, application.Payment.RenewCharger())
	go application.Commission.RunUnfreeze(ctx)
	logger.Info().Msg("worker_running")
	<-ctx.Done()
	logger.Info().Msg("worker_stopped")
}
