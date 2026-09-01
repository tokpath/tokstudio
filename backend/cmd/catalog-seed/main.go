package main

import (
	"context"
	"os"

	"github.com/tokpath/tokstudio/backend/internal/app"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/logx"
)

// catalog-seed 把嵌入的 ofox 公开目录快照预置进 PostgreSQL。
// 不启动 API、不连 Redis/Bifrost。预置模型直接是已审核并已发布。
func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	logger := logx.New(cfg.LogLevel, os.Stdout)
	gdb, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("postgres_open_failed")
	}
	if err := db.Apply(gdb, app.AllMigrations()); err != nil {
		logger.Fatal().Err(err).Msg("migrate_failed")
	}
	result, err := catalog.New(gdb).ImportOfoxSnapshot(context.Background())
	if err != nil {
		logger.Fatal().Err(err).Msg("catalog_seed_failed")
	}
	logger.Info().
		Int("total", result.Total).
		Int("imported", result.Imported).
		Int("deprecated", result.Deprecated).
		Int("skipped", result.Skipped).
		Msg("catalog_seed_ok")
}
