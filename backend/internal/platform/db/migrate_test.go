package db

import (
	"os"
	"sync"
	"testing"
	"testing/fstest"
)

func TestApplyConcurrent(t *testing.T) {
	url := os.Getenv("TOKENHUB_DATABASE_URL")
	if url == "" {
		t.Skip("integration test requires TOKENHUB_DATABASE_URL")
	}
	gdb, err := Open(url)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	fsys := fstest.MapFS{
		"0001_lock_probe.sql": {
			Data: []byte(`CREATE TABLE IF NOT EXISTS migrate_lock_probe (id TEXT PRIMARY KEY);`),
		},
	}
	const workers = 8
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- Apply(gdb, []ModuleMigrations{{Module: "lock_probe", FS: fsys}})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}
