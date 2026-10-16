package db

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/tokpath/tokstudio/backend/internal/platform/db/dbtest"
)

func migrationFS(body string) fstest.MapFS {
	return fstest.MapFS{"0001_init.sql": {Data: []byte(body)}}
}

func TestPlanValidatesDependenciesAndInputOrder(t *testing.T) {
	base := ModuleMigrations{Module: "z_base", FS: migrationFS("SELECT 1")}
	dependent := ModuleMigrations{Module: "a_dependent", FS: migrationFS("SELECT 2"), DependsOn: []string{"z_base"}}
	got, err := plan([]ModuleMigrations{base, dependent})
	if err != nil || len(got) != 2 || got[0].module != "z_base" {
		t.Fatalf("caller order lost: %v %v", got, err)
	}
	if _, err := plan([]ModuleMigrations{dependent, base}); err == nil {
		t.Fatal("missing dependency accepted")
	}
	if _, err := plan([]ModuleMigrations{base, base}); err == nil {
		t.Fatal("duplicate module accepted")
	}
}

func TestApplyConcurrent(t *testing.T) {
	gdb := dbtest.New(t)
	modules := []ModuleMigrations{{Module: "lock_probe", FS: migrationFS(`CREATE TABLE migrate_lock_probe(id TEXT PRIMARY KEY)`)}}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- Apply(gdb, modules) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := Readiness(context.Background(), gdb, modules); err != nil {
		t.Fatal(err)
	}
	var count int64
	gdb.Raw(`SELECT count(*) FROM schema_migrations`).Scan(&count)
	if count != 1 {
		t.Fatalf("applied %d times", count)
	}
}

func TestChangedAndRemovedAppliedFilesAreRejected(t *testing.T) {
	gdb := dbtest.New(t)
	original := []ModuleMigrations{{Module: "test", FS: migrationFS(`CREATE TABLE immutable(id TEXT PRIMARY KEY)`)}}
	if err := Apply(gdb, original); err != nil {
		t.Fatal(err)
	}
	changed := []ModuleMigrations{{Module: "test", FS: migrationFS(`CREATE TABLE immutable(id TEXT PRIMARY KEY); -- changed`)}}
	if err := Apply(gdb, changed); !errors.Is(err, ErrMigrationChanged) {
		t.Fatalf("changed file accepted: %v", err)
	}
	if err := Apply(gdb, []ModuleMigrations{{Module: "test", FS: fstest.MapFS{}}}); !errors.Is(err, ErrMigrationChanged) {
		t.Fatalf("deleted sequence accepted: %v", err)
	}
	if err := Readiness(context.Background(), gdb, changed); !errors.Is(err, ErrMigrationChanged) {
		t.Fatalf("readiness accepted mutation: %v", err)
	}
}

func TestReadinessDoesNotApplyMissingVersions(t *testing.T) {
	gdb := dbtest.New(t)
	sequence := migrationFS(`CREATE TABLE ready_one(id INT)`)
	modules := []ModuleMigrations{{Module: "ready", FS: sequence}}
	if err := Readiness(context.Background(), gdb, modules); !errors.Is(err, ErrSchemaGeneration) {
		t.Fatalf("empty database ready: %v", err)
	}
	if err := Apply(gdb, modules); err != nil {
		t.Fatal(err)
	}
	sequence["0002_next.sql"] = &fstest.MapFile{Data: []byte(`CREATE TABLE ready_two(id INT)`)}
	if err := Readiness(context.Background(), gdb, modules); !errors.Is(err, ErrMigrationMissing) {
		t.Fatalf("missing migration accepted: %v", err)
	}
	var exists bool
	gdb.Raw(`SELECT to_regclass('ready_two') IS NOT NULL`).Scan(&exists)
	if exists {
		t.Fatal("readiness ran migration")
	}
}

func TestLegacyDatabaseAndDifferentGenerationAreRejected(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		gdb := dbtest.New(t)
		if err := gdb.Exec(`CREATE TABLE schema_migrations(module TEXT,version TEXT); CREATE TABLE legacy_customer(id INT)`).Error; err != nil {
			t.Fatal(err)
		}
		if err := Apply(gdb, nil); !errors.Is(err, ErrSchemaGeneration) {
			t.Fatalf("legacy adopted: %v", err)
		}
		var exists bool
		gdb.Raw(`SELECT to_regclass('schema_identity') IS NOT NULL`).Scan(&exists)
		if exists {
			t.Fatal("legacy database marked as new")
		}
	})
	t.Run("generation", func(t *testing.T) {
		gdb := dbtest.New(t)
		if err := Apply(gdb, nil); err != nil {
			t.Fatal(err)
		}
		if err := gdb.Exec(`UPDATE schema_identity SET generation='other-system'`).Error; err != nil {
			t.Fatal(err)
		}
		if err := Apply(gdb, nil); !errors.Is(err, ErrSchemaGeneration) {
			t.Fatalf("generation ignored: %v", err)
		}
	})
}

func TestPlanOrderAndAtomicFailure(t *testing.T) {
	gdb := dbtest.New(t)
	modules := []ModuleMigrations{
		{Module: "z_parent", FS: migrationFS(`CREATE TABLE parent(id INT PRIMARY KEY)`)},
		{Module: "a_child", DependsOn: []string{"z_parent"}, FS: migrationFS(`CREATE TABLE child(id INT REFERENCES parent(id))`)},
	}
	if err := Apply(gdb, modules); err != nil {
		t.Fatal(err)
	}
	fresh := dbtest.New(t)
	modules[1].FS = migrationFS(`CREATE TABLE broken(id INT REFERENCES missing_table(id))`)
	if err := Apply(fresh, modules); err == nil {
		t.Fatal("invalid plan succeeded")
	}
	var exists bool
	fresh.Raw(`SELECT to_regclass('parent') IS NOT NULL OR to_regclass('schema_identity') IS NOT NULL`).Scan(&exists)
	if exists {
		t.Fatal("failed migration left partial schema")
	}
}
