package usage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

func TestExternalFailedAttemptRetriesSameModelAndPreservesAcceptedSupplierPrice(t *testing.T) {
	if os.Getenv("TOKENHUB_MODEL_MOCK_URL") == "" {
		t.Skip("separately deployed model mock required")
	}
	f := setup(t, "100", nil)
	externalRuntime(t, f)
	var row struct{ Definition []byte }
	if e := f.db.Table("tokenhub_upstreams").Select("definition").Where("id='upstream'").Take(&row).Error; e != nil {
		t.Fatal(e)
	}
	var upstream catalog.UpstreamModel
	if e := json.Unmarshal(row.Definition, &upstream); e != nil {
		t.Fatal(e)
	}
	// A definite rejection permits another route. A 503 or disconnected
	// submission may have executed upstream and must remain outcome_unknown.
	upstream.Model = "test-chat-429"
	if e := f.cat.SaveUpstream(f.ctx, f.operator, upstream, 1); e != nil {
		t.Fatal(e)
	}
	upstream.ID, upstream.Model, upstream.Priority = "fallback", "test-chat", 99
	if e := f.cat.SaveUpstream(f.ctx, f.operator, upstream, 0); e != nil {
		t.Fatal(e)
	}
	costSource := f.cat.Costs.(*testCostCatalog)
	changed := false
	ex, e := f.s.Execute(f.ctx, f.key.Key, "retry-at-original-price", request(t, true), func(_ context.Context, event modelruntime.Event) error {
		if !changed && len(event.Data) > 0 {
			changed = true
			// A real streamed result is already in flight when the supplier's
			// new price arrives. Neither its accepted cost nor retail may move.
			costSource.rate = .1
			return nil
		}
		return nil
	})
	if e != nil || !changed || ex.Request.Status != "completed" || ex.Request.ChargedATG.String() != "0.017" {
		t.Fatal("failed route did not recover under the original customer request", ex.Request.Status, ex.Request.ChargedATG, changed, e)
	}
	var attempts []AttemptRecord
	if e = f.db.Where("request_id=?", ex.Request.ID).Order("started_at,id").Find(&attempts).Error; e != nil || len(attempts) != 2 {
		t.Fatal("expected exactly one failed and one successful actual attempt", attempts, e)
	}
	if attempts[0].CostStatus != "pending" || !attempts[0].SupplierCostUSD.IsZero() || attempts[1].CostStatus != "confirmed" || attempts[1].SupplierCostUSD.String() != "0.017" {
		t.Fatal("unknown cost must remain pending; accepted supplier price must remain fixed", attempts)
	}
	replay, e := f.s.Execute(f.ctx, f.key.Key, "retry-at-original-price", request(t, true), func(context.Context, modelruntime.Event) error { return nil })
	if (e != nil && !errors.Is(e, ErrResultUnavailable)) || !replay.Replay {
		t.Fatal("replay repeated the generation", replay, e)
	}
	var count int64
	if e = f.db.Model(&AttemptRecord{}).Where("request_id=?", ex.Request.ID).Count(&count).Error; e != nil || count != 2 {
		t.Fatal("replay created another provider attempt", count, e)
	}
	grants, e := f.l.CustomerGrants(f.ctx, nil, f.operator, f.customer)
	if e != nil || len(grants) != 1 {
		t.Fatal(grants, e)
	}
	account, e := f.l.AccountInTransaction(f.ctx, nil, grants[0].AccountID)
	if e != nil || account.Balance.String() != "99.983" || !account.Held.IsZero() {
		t.Fatal("internal retry charged twice or retained a hold", account, e)
	}
	if e = f.l.Reconcile(f.ctx, nil, account.ID); e != nil {
		t.Fatal(e)
	}
}
