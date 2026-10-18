package operations

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPostgresExportAcceptsEveryAdvertisedBusinessKind(t *testing.T) {
	s, actor, _, operator, _ := opsFixture(t)
	ctx := context.Background()
	for _, kind := range []string{"ledger", "usage", "requests", "procurement"} {
		out, err := s.RequestExport(ctx, actor.actor, operator, ExportQuery{Kind: kind, From: time.Now().Add(-time.Hour), To: time.Now()})
		if err != nil || out.Kind != kind || out.State != "pending" {
			t.Fatal("valid advertised export was rejected", kind, out, err)
		}
	}
	if _, err := s.RequestExport(ctx, actor.actor, operator, ExportQuery{Kind: "other_tenant", From: time.Now().Add(-time.Hour), To: time.Now()}); !errors.Is(err, ErrInvalid) {
		t.Fatal("unknown export kind accepted", err)
	}
}
