package main

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/logx"
	"github.com/tokpath/tokstudio/backend/internal/portal"
	"os"
)

func main() {
	cfg, err := portal.LoadConfig()
	if err != nil {
		panic(err)
	}
	logger := logx.New(cfg.LogLevel, os.Stdout)
	gdb, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("postgres_open_failed")
	}
	if err = db.Apply(gdb, portal.AllMigrations()); err != nil {
		logger.Fatal().Err(err).Msg("migration_failed")
	}
	if cfg.S3Bucket != "" {
		store, e := portal.NewObjectStore(context.Background(), cfg)
		if e != nil {
			logger.Fatal().Err(e).Msg("object_store_configuration_failed")
		}
		if e = store.Ready(context.Background()); e != nil {
			logger.Fatal().Err(e).Msg("object_store_not_ready")
		}
		if e = store.ConfigureTemporaryLifecycle(context.Background()); e != nil {
			logger.Fatal().Err(e).Msg("object_store_lifecycle_failed")
		}
	}
	installed, err := portal.Bootstrap(context.Background(), cfg, gdb)
	if err != nil {
		logger.Fatal().Err(err).Msg("bootstrap_failed")
	}
	logger.Info().Str("operator_id", installed.OperatorID).Str("brand_id", installed.BrandID).Msg("migration_and_bootstrap_complete")
}
