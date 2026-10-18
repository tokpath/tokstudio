package portal

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/tokpath/tokstudio/backend/internal/jobs"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This helper attaches to its parent's isolated, already-migrated schema. It
// executes the actual discovery/lease/maintenance/notification worker loop in a
// separate OS process. The synthetic mailbox records SMTP submission only.
func TestRestrictionReviewWorkerProcessHelper(t *testing.T) {
	schema := os.Getenv("TOKENHUB_REVIEW_WORKER_SCHEMA")
	if schema == "" {
		t.Skip("private independent worker helper")
	}
	if !regexp.MustCompile(`^infra_[0-9a-f]{32}$`).MatchString(schema) {
		t.Fatal("isolated parent schema required")
	}
	pg, e := pgx.ParseConfig(os.Getenv("TOKENHUB_DATABASE_URL"))
	if e != nil {
		t.Fatal("invalid isolated database connection")
	}
	pg.RuntimeParams["search_path"] = schema
	connection := stdlib.OpenDB(*pg)
	defer connection.Close()
	gdb, e := gorm.Open(postgres.New(postgres.Config{Conn: connection}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(os.Getenv("TOKENHUB_REVIEW_WORKER_CONFIG"))
	if e != nil {
		t.Fatal("private parent configuration unavailable")
	}
	var cfg Config
	if json.Unmarshal(raw, &cfg) != nil {
		t.Fatal("invalid private parent configuration")
	}
	a, e := New(context.Background(), cfg, gdb)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	box := &mailbox{}
	a.Mail.SetSender(box)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.RunWorker(ctx, 2, func(kind string, _ error) { t.Log("worker dependency requires review:", kind) })
	}()
	defer func() { cancel(); <-done }()
	expected, e := strconv.Atoi(os.Getenv("TOKENHUB_REVIEW_WORKER_EXPECT_SENT"))
	if e != nil {
		t.Fatal("expected submission count required")
	}
	jobID := os.Getenv("TOKENHUB_REVIEW_WORKER_JOB")
	for {
		var job jobs.Job
		if e = gdb.Where("id=?", jobID).First(&job).Error; e != nil {
			t.Fatal(e)
		}
		var accepted int64
		if e = gdb.Table("operations_deliveries d").Joins("JOIN operations_notifications n ON n.id=d.notification_id").Where("n.business_key LIKE 'restriction-review:%' AND d.state='accepted'").Count(&accepted).Error; e != nil {
			t.Fatal(e)
		}
		if job.Status == "completed" && accepted == 1 {
			cancel()
			<-done
			box.mu.Lock()
			sent := len(box.messages)
			box.mu.Unlock()
			if sent != expected {
				t.Fatal("independent worker submitted duplicate or missing mail", sent, expected)
			}
			t.Logf("independent worker: maintenance=%s status=%s accepted=%d new_submissions=%d", jobID, job.Status, accepted, sent)
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("independent worker did not finish restriction review", job.Status, accepted)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func TestPostgresRestrictionReviewIndependentWorkerAndRestartReplay(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	op := f.a.Installation.OperatorID
	reviewDecision(t, f, f.a.Installation.OwnerStaffID, op, "operator", op, "model_usage", -time.Hour)
	if e := f.a.DiscoverWorkerTasks(ctx); e != nil {
		t.Fatal(e)
	}
	var original jobs.Job
	if e := f.a.DB.Where("queue='portal-maintenance' AND kind='restriction_reviews'").First(&original).Error; e != nil {
		t.Fatal("automatic discovery omitted restriction review", e)
	}
	var schema string
	if e := f.a.DB.Raw("SELECT current_schema()").Row().Scan(&schema); e != nil {
		t.Fatal(e)
	}
	configPath := filepath.Join(t.TempDir(), "private-worker-config.json")
	configuration, e := json.Marshal(f.a.Config)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(configPath, configuration, 0600); e != nil {
		t.Fatal(e)
	}
	run := func(job string, expectedSent int) {
		t.Helper()
		command := exec.Command(os.Args[0], "-test.run=^TestRestrictionReviewWorkerProcessHelper$", "-test.timeout=35s", "-test.v")
		command.Env = append(os.Environ(), "TOKENHUB_REVIEW_WORKER_SCHEMA="+schema, "TOKENHUB_REVIEW_WORKER_CONFIG="+configPath, "TOKENHUB_REVIEW_WORKER_JOB="+job, "TOKENHUB_REVIEW_WORKER_EXPECT_SENT="+strconv.Itoa(expectedSent))
		output, e := command.CombinedOutput()
		if e != nil {
			t.Fatalf("independent worker: %v %s", e, output)
		}
		t.Log(string(output))
	}
	run(original.ID, 1)
	notices := reviewNotices(t, f, op)
	if len(notices) != 1 || notices[0].Severity != "warning" {
		t.Fatal("worker failed to persist exact review warning", notices)
	}
	// A replacement process receives a separately leased retry of the same
	// business scan. The original notification and SMTP acceptance must stand.
	replay, e := f.a.Jobs.Enqueue(ctx, jobs.Input{Queue: "portal-maintenance", Kind: "restriction_reviews", BusinessKey: "independent-review-replay:" + uuid.NewString(), Payload: original.Payload, MaxAttempts: 12})
	if e != nil {
		t.Fatal(e)
	}
	run(replay.ID, 0)
	final := reviewNotices(t, f, op)
	if len(final) != 1 || final[0].ID != notices[0].ID {
		t.Fatal("replacement process duplicated the review notification", final)
	}
	delivery, e := f.a.Operations.GetDelivery(ctx, nil, final[0].ID)
	if e != nil || delivery.State != "accepted" || delivery.Attempts != 1 {
		t.Fatal("replacement process repeated accepted SMTP submission", delivery, e)
	}
	t.Logf("A112 independent OS worker evidence: notification=%s original_job=%s replay_job=%s accepted_once=true", final[0].ID, original.ID, replay.ID)
}
