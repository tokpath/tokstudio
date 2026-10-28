package operations

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"testing"
	"time"
)

type exportWithoutReports struct{ Authorizer }

func (a exportWithoutReports) RequirePermission(ctx context.Context, tx *gorm.DB, actor, operator, permission string) error {
	if permission == "reports.view" {
		return ErrForbidden
	}
	return a.Authorizer.RequirePermission(ctx, tx, actor, operator, permission)
}

func TestPostgresFinancialExportRequiresCurrentReportPermission(t *testing.T) {
	s, actor, _, operator, _ := opsFixture(t)
	ctx := context.Background()
	query := ExportQuery{Kind: "financial", From: time.Now().Add(-time.Hour), To: time.Now()}
	job, err := s.RequestExport(ctx, actor.actor, operator, query)
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := s.seal([]byte("分类,指标,数值,单位\n经营结果,收入合计,1,USD\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DB.Model(&Export{}).Where("id=?", job.ID).Updates(map[string]any{"state": "ready", "content_ciphertext": cipher}).Error; err != nil {
		t.Fatal(err)
	}
	s.Authorize = exportWithoutReports{s.Authorize}
	if _, err = s.DownloadExport(ctx, actor.actor, operator, job.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("revoked report access still downloaded", err)
	}
	if _, err = s.RequestExport(ctx, actor.actor, operator, query); !errors.Is(err, ErrForbidden) {
		t.Fatal("export permission alone exposed report", err)
	}
	query.Kind = "ledger"
	if _, err = s.RequestExport(ctx, actor.actor, operator, query); err != nil {
		t.Fatal("unrelated exports were denied", err)
	}
}

func TestPostgresExportAcceptsEveryAdvertisedBusinessKind(t *testing.T) {
	s, actor, _, operator, _ := opsFixture(t)
	ctx := context.Background()
	for _, kind := range []string{"ledger", "financial", "orders", "payments", "requests", "procurement"} {
		out, err := s.RequestExport(ctx, actor.actor, operator, ExportQuery{Kind: kind, From: time.Now().Add(-time.Hour), To: time.Now()})
		if err != nil || out.Kind != kind || out.State != "pending" {
			t.Fatal("valid advertised export was rejected", kind, out, err)
		}
	}
	if _, err := s.RequestExport(ctx, actor.actor, operator, ExportQuery{Kind: "other_tenant", From: time.Now().Add(-time.Hour), To: time.Now()}); !errors.Is(err, ErrInvalid) {
		t.Fatal("unknown export kind accepted", err)
	}
}
