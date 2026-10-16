package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/platform/backup"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
)

func databaseCommand(ctx context.Context, dsn, command string, args ...string) ([]byte, error) {
	u, e := url.Parse(dsn)
	if e != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" {
		return nil, errors.New("backup requires a PostgreSQL connection URL")
	}
	c := exec.CommandContext(ctx, command, args...)
	// libpq does not expand a connection URL from PGDATABASE. Set individual
	// variables, keeping passwords out of argv and removing inherited overrides.
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "PG") {
			c.Env = append(c.Env, v)
		}
	}
	c.Env = append(c.Env, "PGHOST="+u.Hostname(), "PGDATABASE="+strings.TrimPrefix(u.Path, "/"))
	if p := u.Port(); p != "" {
		c.Env = append(c.Env, "PGPORT="+p)
	}
	if u.User != nil {
		c.Env = append(c.Env, "PGUSER="+u.User.Username())
		if p, ok := u.User.Password(); ok {
			c.Env = append(c.Env, "PGPASSWORD="+p)
		}
	}
	options := map[string]string{"sslmode": "PGSSLMODE", "sslrootcert": "PGSSLROOTCERT", "sslcert": "PGSSLCERT", "sslkey": "PGSSLKEY", "sslpassword": "PGSSLPASSWORD", "sslcrl": "PGSSLCRL", "connect_timeout": "PGCONNECT_TIMEOUT", "application_name": "PGAPPNAME", "options": "PGOPTIONS", "target_session_attrs": "PGTARGETSESSIONATTRS", "channel_binding": "PGCHANNELBINDING"}
	for k, values := range u.Query() {
		name, ok := options[k]
		if !ok || len(values) != 1 {
			return nil, errors.New("unsupported PostgreSQL backup URL option")
		}
		c.Env = append(c.Env, name+"="+values[0])
	}
	b, e := c.Output()
	if e != nil {
		return nil, fmt.Errorf("%s failed; inspect database reachability and permissions", command)
	}
	return b, nil
}
func scalar(ctx context.Context, dsn, query string) (string, error) {
	b, e := databaseCommand(ctx, dsn, "psql", "--no-psqlrc", "--set=ON_ERROR_STOP=1", "--tuples-only", "--no-align", "--command", query)
	return strings.TrimSpace(string(b)), e
}
func newStore(ctx context.Context, c *config.Config) (*objectstore.Store, error) {
	return objectstore.New(ctx, objectstore.Settings{Endpoint: c.S3Endpoint, Region: c.S3Region, Bucket: c.S3Bucket, AccessKey: c.S3AccessKey, SecretKey: c.S3SecretKey, ForcePathStyle: c.S3ForcePathStyle})
}
func run() error {
	mode := flag.String("mode", "", "create, open, inspect or restore-drill")
	input := flag.String("input", "", "encrypted backup input, or verified archive for inspect")
	output := flag.String("output", "", "new backup or decrypted archive file (never overwrite)")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	if *mode == "inspect" {
		m, e := backup.InspectArchive(*input)
		if e != nil {
			return e
		}
		fmt.Printf("verified archive: generation=%s objects=%d created_at=%s\n", m.Generation, len(m.Files)-1, m.CreatedAt.Format(time.RFC3339))
		return nil
	}
	c, e := config.Load()
	if e != nil {
		return e
	}
	key := os.Getenv("TOKENHUB_BACKUP_KEY")
	if len(key) < 32 {
		return errors.New("TOKENHUB_BACKUP_KEY must contain at least 32 random characters")
	}
	if *mode == "open" {
		return backup.TransformFile(*input, *output, key, true)
	}
	tmp, e := os.MkdirTemp("", "tokenhub-backup-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	store, e := newStore(ctx, c)
	if e != nil {
		return e
	}
	switch *mode {
	case "create":
		if *output == "" {
			return errors.New("new output filename required")
		}
		generation, e := scalar(ctx, c.DatabaseURL, "SELECT generation FROM schema_identity WHERE id=TRUE")
		if e != nil {
			return e
		}
		if generation != "tokenhub-operator-v2" {
			return errors.New("only the rebuilt schema generation can be backed up")
		}
		dump := filepath.Join(tmp, "database.dump")
		if _, e = databaseCommand(ctx, c.DatabaseURL, "pg_dump", "--format=custom", "--no-owner", "--no-privileges", "--file", dump); e != nil {
			return e
		}
		archive := filepath.Join(tmp, "archive.tar")
		if e = backup.PublishFile(archive, func(w io.Writer) error { return backup.BuildArchive(ctx, dump, store, w) }); e != nil {
			return e
		}
		if _, e = backup.InspectArchive(archive); e != nil {
			return e
		}
		if e = backup.TransformFile(archive, *output, key, false); e != nil {
			return e
		}
		fmt.Println("authenticated backup created; excludes temporary model outputs and regenerated exports")
		return nil
	case "restore-drill":
		target := os.Getenv("TOKENHUB_RESTORE_DATABASE_URL")
		u, e := url.Parse(target)
		if e != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "tokenhub_restore_drill_") {
			return errors.New("restore requires an explicit isolated tokenhub_restore_drill_* database URL")
		}
		if target == c.DatabaseURL {
			return errors.New("restore target cannot be source database")
		}
		count, e := scalar(ctx, target, "SELECT count(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog','information_schema')")
		if e != nil {
			return e
		}
		if count != "0" {
			return errors.New("restore requires an empty database; existing data is never cleaned")
		}
		archive := filepath.Join(tmp, "archive.tar")
		if e = backup.TransformFile(*input, archive, key, true); e != nil {
			return e
		}
		dump := filepath.Join(tmp, "database.dump")
		m, e := backup.RestoreArchive(ctx, archive, dump, store)
		if e != nil {
			return e
		}
		if _, e = databaseCommand(ctx, target, "pg_restore", "--dbname="+strings.TrimPrefix(u.Path, "/"), "--exit-on-error", "--single-transaction", "--no-owner", "--no-privileges", dump); e != nil {
			return e
		}
		generation, e := scalar(ctx, target, "SELECT generation FROM schema_identity WHERE id=TRUE")
		if e != nil || generation != m.Generation {
			return errors.New("restored database generation mismatch")
		}
		check := `SELECT count(*) FROM (
 SELECT j.id FROM ledger_journals j LEFT JOIN ledger_entries e ON e.journal_id=j.id GROUP BY j.id,e.unit HAVING count(e.id)=0 OR sum(e.amount)<>0
 UNION ALL SELECT a.id FROM ledger_accounts a LEFT JOIN (SELECT account_id,sum(amount) amount FROM ledger_entries GROUP BY account_id)e ON e.account_id=a.id WHERE a.balance<>coalesce(e.amount,0)
 UNION ALL SELECT a.id FROM ledger_accounts a LEFT JOIN (SELECT ha.account_id,sum(ha.amount) amount FROM ledger_hold_allocations ha JOIN ledger_holds h ON h.id=ha.hold_id WHERE h.status='active' GROUP BY ha.account_id)h ON h.account_id=a.id WHERE a.held<>coalesce(h.amount,0)
) discrepancies`
		bad, e := scalar(ctx, target, check)
		if e != nil {
			return e
		}
		if n, e := strconv.Atoi(bad); e != nil || n != 0 {
			return errors.New("restored ledger failed balance/reservation reconciliation")
		}
		fmt.Printf("restore verified: generation=%s durable_objects=%d ledger_discrepancies=0; isolated database and bucket retained\n", m.Generation, len(m.Files)-1)
		fmt.Println("No worker started. Verify external payment facts since the backup and reconcile pending jobs before any operational recovery.")
		return nil
	default:
		return errors.New("choose --mode=create, open, inspect or restore-drill")
	}
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
