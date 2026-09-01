package db

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// migrateLockKey serializes Apply across api + worker + parallel tests.
// PostgreSQL CREATE TABLE IF NOT EXISTS still races on pg_type.
const migrateLockKey int64 = 8817260142

// ModuleMigrations 是一个模块自己的 SQL 迁移目录。
// 模块不得读取其他模块的表或复用内部 ORM Model。
type ModuleMigrations struct {
	Module string
	FS     fs.FS
}

// Apply 按模块名、文件名顺序执行版本化 SQL。已执行的版本跳过。
func Apply(gdb *gorm.DB, modules []ModuleMigrations) error {
	// One transaction pins a connection so the advisory lock is held on the
	// same session that runs CREATE TABLE. Session-level pg_advisory_lock
	// would leak across GORM's pool.
	return gdb.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(?)`, migrateLockKey).Error; err != nil {
			return fmt.Errorf("migrate lock: %w", err)
		}
		if err := tx.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
    module TEXT NOT NULL,
    version TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (module, version)
)`).Error; err != nil {
			return fmt.Errorf("create schema_migrations: %w", err)
		}

		for _, module := range modules {
			entries, err := fs.ReadDir(module.FS, ".")
			if err != nil {
				return fmt.Errorf("read migrations for %s: %w", module.Module, err)
			}
			names := make([]string, 0, len(entries))
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
					continue
				}
				names = append(names, entry.Name())
			}
			sort.Strings(names)
			for _, name := range names {
				version := strings.TrimSuffix(name, ".sql")
				var count int64
				if err := tx.Raw(
					`SELECT COUNT(1) FROM schema_migrations WHERE module = ? AND version = ?`,
					module.Module, version,
				).Scan(&count).Error; err != nil {
					return err
				}
				if count > 0 {
					continue
				}
				body, err := fs.ReadFile(module.FS, path.Clean(name))
				if err != nil {
					return fmt.Errorf("read %s/%s: %w", module.Module, name, err)
				}
				if err := tx.Exec(string(body)).Error; err != nil {
					return fmt.Errorf("apply %s/%s: %w", module.Module, name, err)
				}
				if err := tx.Exec(
					`INSERT INTO schema_migrations(module, version) VALUES (?, ?)`,
					module.Module, version,
				).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Applied 返回已执行的模块版本，供就绪检查使用。
func Applied(gdb *gorm.DB) (map[string][]string, error) {
	type row struct {
		Module  string
		Version string
	}
	var rows []row
	if err := gdb.Raw(`SELECT module, version FROM schema_migrations ORDER BY module, version`).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, item := range rows {
		out[item.Module] = append(out[item.Module], item.Version)
	}
	return out, nil
}
