package portal

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func TestPostgresListingCoverageDoesNotTraverseUnrelatedOperators(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	child, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{
		ActorStaffID: f.a.Installation.OwnerStaffID, ParentOperatorID: f.a.Installation.OperatorID,
		Kind: organization.Channel, Name: "Unconfigured independent channel", LegalName: "Synthetic operator",
		QuoteReference: "synthetic supply terms", ExitTermsReference: "synthetic exit terms",
	})
	if err != nil {
		t.Fatal(err)
	}
	model := f.configureExternalCatalog(t, f.server.URL)
	if err = f.a.Organization.SetSupplyStatus(ctx, nil, f.a.Installation.OwnerStaffID, child.Operator.ID, "active", "synthetic agreement"); err != nil {
		t.Fatal(err)
	}
	if err = f.a.Catalog.SetAuthorization(ctx, f.a.Installation.OperatorID, child.Operator.ID, model, true, 0); err != nil {
		t.Fatal(err)
	}
	if err = f.a.Catalog.SetListing(ctx, child.Operator.ID, model, true, 0); err != nil {
		t.Fatal(err)
	}
	// An incomplete channel cannot pass its own coverage check, but its setup
	// must not gate a listing used only by the platform's direct customers.
	payload, _ := json.Marshal(map[string]any{"enabled": true, "model_id": model})
	change := catalog.CatalogChange{Kind: "listing", EffectiveAt: time.Now().UTC(), Payload: payload}
	change.OperatorID = child.Operator.ID
	if err = f.a.DB.Transaction(func(tx *gorm.DB) error { return f.a.validateCatalogChange(ctx, tx, change) }); err == nil {
		t.Fatal("unconfigured channel unexpectedly passed its own coverage check")
	}
	change.OperatorID = f.a.Installation.OperatorID
	if err = f.a.DB.Transaction(func(tx *gorm.DB) error { return f.a.validateCatalogChange(ctx, tx, change) }); err != nil {
		t.Fatal("independent child blocked platform retail listing:", err)
	}
	// Changes to inherited authorization or the public model still affect the
	// child, and must retain its coverage check.
	change.Kind = "authorization"
	change.Payload, _ = json.Marshal(map[string]any{"buyer_id": child.Operator.ID, "model_id": model, "enabled": false})
	if err = f.a.DB.Transaction(func(tx *gorm.DB) error { return f.a.validateCatalogChange(ctx, tx, change) }); err == nil {
		t.Fatal("authorization change skipped affected child coverage")
	}
	change.Kind = "model"
	if err = f.a.DB.Transaction(func(tx *gorm.DB) error { return f.a.validateCatalogChange(ctx, tx, change) }); err == nil {
		t.Fatal("global model change skipped descendant coverage")
	}
}
