package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// SchemaGeneration prevents the rebuilt business model from silently adopting a
// database created by the old application. Changing a generation requires a new,
// isolated database, never an automatic destructive reset.
const SchemaGeneration = "tokenhub-operator-v2"
const migrateLockKey int64 = 8817260142

var (
	ErrSchemaGeneration = errors.New("incompatible schema generation; use an isolated empty database")
	ErrMigrationChanged = errors.New("applied migration content changed")
	ErrMigrationMissing = errors.New("required migration is not applied")
)

// ModuleMigrations is one module's immutable SQL sequence. Apply follows the
// caller's module order exactly; filenames are sorted within each module.
// DependsOn, when supplied, must name modules earlier in that same plan.
type ModuleMigrations struct {
	Module    string
	FS        fs.FS
	DependsOn []string
}

type migration struct {
	module, version, checksum, body string
}

type appliedMigration struct {
	Module, Version, Checksum string
}

func plan(modules []ModuleMigrations) ([]migration, error) {
	seen := map[string]bool{}
	var result []migration
	for _, module := range modules {
		if strings.TrimSpace(module.Module) == "" || seen[module.Module] || module.FS == nil {
			return nil, fmt.Errorf("invalid or duplicate migration module %q", module.Module)
		}
		for _, dependency := range module.DependsOn {
			if !seen[dependency] {
				return nil, fmt.Errorf("migration module %s requires %s earlier in the plan", module.Module, dependency)
			}
		}
		seen[module.Module] = true
		entries, err := fs.ReadDir(module.FS, ".")
		if err != nil {
			return nil, fmt.Errorf("read migrations for %s: %w", module.Module, err)
		}
		var names []string
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
				names = append(names, entry.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			body, err := fs.ReadFile(module.FS, name)
			if err != nil {
				return nil, fmt.Errorf("read %s/%s: %w", module.Module, name, err)
			}
			digest := sha256.Sum256(body)
			result = append(result, migration{module.Module, strings.TrimSuffix(name, ".sql"), hex.EncodeToString(digest[:]), string(body)})
		}
	}
	return result, nil
}

// Apply is an explicit deployment operation, not an API/worker startup side
// effect. The whole plan commits atomically under a transaction advisory lock.
// Applied files are verified, including detection of removed versions.
func Apply(gdb *gorm.DB, modules []ModuleMigrations) error {
	migrations, err := plan(modules)
	if err != nil {
		return err
	}
	return gdb.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(?)`, migrateLockKey).Error; err != nil {
			return fmt.Errorf("migrate lock: %w", err)
		}
		if err := initializeGeneration(tx); err != nil {
			return err
		}
		if err := tx.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
			module TEXT NOT NULL, version TEXT NOT NULL,
			checksum TEXT NOT NULL CHECK (length(checksum) = 64),
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (module, version))`).Error; err != nil {
			return fmt.Errorf("create schema_migrations: %w", err)
		}
		applied, err := loadApplied(tx)
		if err != nil {
			return err
		}
		if err := verifyApplied(migrations, modules, applied, false); err != nil {
			return err
		}
		for _, item := range migrations {
			if _, exists := applied[item.module+"/"+item.version]; exists {
				continue
			}
			if err := tx.Exec(item.body).Error; err != nil {
				return fmt.Errorf("apply %s/%s: %w", item.module, item.version, err)
			}
			if err := tx.Exec(`INSERT INTO schema_migrations(module,version,checksum) VALUES (?,?,?)`, item.module, item.version, item.checksum).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func initializeGeneration(tx *gorm.DB) error {
	var markerExists bool
	if err := tx.Raw(`SELECT to_regclass('schema_identity') IS NOT NULL`).Scan(&markerExists).Error; err != nil {
		return err
	}
	if markerExists {
		return checkGeneration(tx)
	}
	// Refuse even an empty old schema_migrations table or unrelated application
	// table. Only a truly empty schema may receive a new generation marker.
	var tables int64
	if err := tx.Raw(`SELECT count(*) FROM pg_catalog.pg_class
		WHERE relnamespace = current_schema()::regnamespace AND relkind IN ('r','p','v','m','S')`).Scan(&tables).Error; err != nil {
		return err
	}
	if tables != 0 {
		return fmt.Errorf("%w: unmarked schema contains %d relations", ErrSchemaGeneration, tables)
	}
	if err := tx.Exec(`CREATE TABLE schema_identity (
		id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
		generation TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now())`).Error; err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO schema_identity(id,generation) VALUES (TRUE,?)`, SchemaGeneration).Error
}

func checkGeneration(tx *gorm.DB) error {
	var generations []string
	if err := tx.Raw(`SELECT generation FROM schema_identity WHERE id = TRUE`).Scan(&generations).Error; err != nil {
		return fmt.Errorf("%w: %v", ErrSchemaGeneration, err)
	}
	if len(generations) != 1 || generations[0] != SchemaGeneration {
		return fmt.Errorf("%w: expected %s", ErrSchemaGeneration, SchemaGeneration)
	}
	return nil
}

func loadApplied(gdb *gorm.DB) (map[string]appliedMigration, error) {
	var rows []appliedMigration
	if err := gdb.Raw(`SELECT module,version,checksum FROM schema_migrations`).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	result := make(map[string]appliedMigration, len(rows))
	for _, row := range rows {
		result[row.Module+"/"+row.Version] = row
	}
	return result, nil
}

func verifyApplied(migrations []migration, selected []ModuleMigrations, applied map[string]appliedMigration, requireAll bool) error {
	expected := map[string]migration{}
	modules := map[string]bool{}
	for _, module := range selected {
		modules[module.Module] = true
	}
	for _, item := range migrations {
		key := item.module + "/" + item.version
		expected[key] = item
		modules[item.module] = true
		row, found := applied[key]
		if found && row.Checksum != item.checksum {
			return fmt.Errorf("%w: %s", ErrMigrationChanged, key)
		}
		if !found && requireAll {
			return fmt.Errorf("%w: %s", ErrMigrationMissing, key)
		}
	}
	for key, row := range applied {
		if _, found := expected[key]; !found && (requireAll || modules[row.Module]) {
			return fmt.Errorf("%w: %s is absent from the migration plan", ErrMigrationChanged, key)
		}
	}
	return nil
}

// Readiness checks the exact generation and complete migration plan without
// applying SQL. An older or newer application cannot accidentally serve against
// a different schema. Call with the full app plan, not a module subset.
func Readiness(ctx context.Context, gdb *gorm.DB, modules []ModuleMigrations) error {
	migrations, err := plan(modules)
	if err != nil {
		return err
	}
	return gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY`).Error; err != nil {
			return err
		}
		if err := checkGeneration(tx); err != nil {
			return err
		}
		applied, err := loadApplied(tx)
		if err != nil {
			return err
		}
		return verifyApplied(migrations, modules, applied, true)
	})
}

// Applied returns migration versions for operational inspection.
func Applied(gdb *gorm.DB) (map[string][]string, error) {
	type row struct{ Module, Version string }
	var rows []row
	if err := gdb.Raw(`SELECT module,version FROM schema_migrations ORDER BY module,version`).Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := map[string][]string{}
	for _, item := range rows {
		result[item.Module] = append(result[item.Module], item.Version)
	}
	return result, nil
}
