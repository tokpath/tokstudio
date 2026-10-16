package jobs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The helper runs as a separate OS process and attaches only to the parent
// test's randomly named schema. It never creates, drops or migrates a database.
func TestJobWorkerProcessHelper(t *testing.T) {
	mode := os.Getenv("TOKENHUB_JOB_CRASH_HELPER")
	if mode == "" {
		t.Skip("private subprocess helper")
	}
	schema := os.Getenv("TOKENHUB_JOB_CRASH_SCHEMA")
	if !regexp.MustCompile(`^infra_[0-9a-f]{32}$`).MatchString(schema) {
		t.Fatal("isolated parent test schema required")
	}
	cfg, err := pgx.ParseConfig(os.Getenv("TOKENHUB_DATABASE_URL"))
	if err != nil {
		t.Fatal("invalid isolated test connection")
	}
	cfg.RuntimeParams["search_path"] = schema
	conn := stdlib.OpenDB(*cfg)
	defer conn.Close()
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	s := New(gdb)
	owner := "process-" + mode
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var job Job
	for {
		claims, err := s.Claim(ctx, "default", owner, 2*time.Second, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(claims) == 1 {
			job = claims[0]
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("lease was not recovered")
		case <-time.After(50 * time.Millisecond):
		}
	}
	write := func(tx *gorm.DB) error {
		return tx.Exec(`INSERT INTO crash_effects(job_id) VALUES (?) ON CONFLICT DO NOTHING`, job.ID).Error
	}
	if mode == "complete" {
		if err = gdb.Transaction(func(tx *gorm.DB) error {
			if err := write(tx); err != nil {
				return err
			}
			return s.CompleteTx(tx, job.ID, owner, job.Fence)
		}); err != nil {
			t.Fatal(err)
		}
		return
	}
	if mode == "effect_committed" {
		if err = gdb.Transaction(write); err != nil {
			t.Fatal(err)
		}
	} else if mode == "uncommitted" {
		tx := gdb.Begin()
		defer tx.Rollback()
		if err = write(tx); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Fatal("unsupported helper mode")
	}
	fmt.Println("CRASH_POINT_READY")
	// Parent sends SIGKILL after this exact durable boundary. No graceful
	// cancellation or synthetic clock update is used to reclaim the lease.
	<-ctx.Done()
	t.Fatal("parent did not kill the worker")
}

func TestPostgresActualProcessLossReclaimsLeaseWithoutDuplicateEffect(t *testing.T) {
	for _, mode := range []string{"uncommitted", "effect_committed"} {
		t.Run(mode, func(t *testing.T) {
			gdb, s := setup(t)
			if err := gdb.Exec(`CREATE TABLE crash_effects(job_id TEXT PRIMARY KEY)`).Error; err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			job, err := s.Enqueue(ctx, Input{Kind: "delivery", BusinessKey: "real-process-loss"})
			if err != nil {
				t.Fatal(err)
			}
			var schema string
			if err := gdb.Raw("SELECT current_schema()").Row().Scan(&schema); err != nil {
				t.Fatal(err)
			}
			command := func(mode string) *exec.Cmd {
				cmd := exec.Command(os.Args[0], "-test.run=^TestJobWorkerProcessHelper$", "-test.timeout=20s")
				cmd.Env = append(os.Environ(), "TOKENHUB_JOB_CRASH_HELPER="+mode, "TOKENHUB_JOB_CRASH_SCHEMA="+schema)
				return cmd
			}
			child := command(mode)
			stdout, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err = child.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = child.Process.Kill() })
			ready := make(chan bool, 1)
			go func() {
				scanner := bufio.NewScanner(stdout)
				for scanner.Scan() {
					if strings.TrimSpace(scanner.Text()) == "CRASH_POINT_READY" {
						ready <- true
						return
					}
				}
				ready <- false
			}()
			select {
			case ok := <-ready:
				if !ok {
					t.Fatal("child exited before crash boundary")
				}
			case <-time.After(15 * time.Second):
				t.Fatal("child did not reach crash boundary")
			}
			var old Job
			if err := gdb.First(&old, "id=?", job.ID).Error; err != nil {
				t.Fatal(err)
			}
			if err := child.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			if err := child.Wait(); err == nil {
				t.Fatal("worker did not lose its process")
			}
			if output, err := command("complete").CombinedOutput(); err != nil {
				t.Fatalf("replacement worker: %v %s", err, output)
			}
			var finished Job
			if err := gdb.First(&finished, "id=?", job.ID).Error; err != nil || finished.Status != "completed" || finished.Fence <= old.Fence || finished.Attempts != 2 {
				t.Fatal("replacement did not complete original job", finished, err)
			}
			if err := s.Complete(ctx, job.ID, old.LeaseOwner, old.Fence); !errors.Is(err, ErrLeaseLost) {
				t.Fatal("dead process fence could complete again", err)
			}
			var count int64
			if err := gdb.Table("crash_effects").Count(&count).Error; err != nil || count != 1 {
				t.Fatal("effect was lost or duplicated", count, err)
			}
		})
	}
}
