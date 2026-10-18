package cooperation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestQuoteExitPolicyRejectsContradictoryInstructions(t *testing.T) {
	for _, test := range []struct{ policy, raw, want string }{
		{"", "", "non_refundable"},
		{"remaining_paid_proportional", "", "remaining_paid_proportional"},
		{"", `{"method":"remaining_paid_proportional","voluntary_refund":true}`, "remaining_paid_proportional"},
		{"non_refundable", `{"method":"remaining_paid_proportional","voluntary_refund":true}`, ""},
		{"automatic_refund", "", ""},
		{"", `{"method":"non_refundable","voluntary_refund":true}`, ""},
	} {
		got, err := quoteExitPolicy(test.policy, json.RawMessage(test.raw))
		if test.want == "" {
			if !errors.Is(err, ErrExitTerms) {
				t.Fatalf("accepted contradictory policy %+v: %s %v", test, got, err)
			}
		} else if err != nil || got != test.want {
			t.Fatalf("policy %+v: %s %v", test, got, err)
		}
	}
}

func TestPostgresReadableQuoteGeneratesReferencesAndFreezesAcceptedRules(t *testing.T) {
	f := setup(t)
	invite, err := f.org.InviteStaff(f.ctx, nil, f.owner, f.channel, "readable-buyer@example.test", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.org.AcceptStaffInvitation(f.ctx, nil, invite.Token, organization.VerifiedStaff{ID: f.owner, Email: "readable-buyer@example.test", EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	publish := func(policy string) PurchaseQuote {
		t.Helper()
		q, err := f.s.PublishPurchaseQuote(f.ctx, nil, f.owner, PurchaseQuote{SupplierID: f.platform, BuyerID: f.channel, Name: "日常采购", TermsSummary: "按实际到账交付", PriceUSD: amount("10"), PaidATG: amount("100"), GiftATG: amount("20"), ExitPolicy: policy, Enabled: true, TermsReference: "client-controlled-reference", Version: 999})
		if err != nil {
			t.Fatal(err)
		}
		return q
	}
	q := publish("remaining_paid_proportional")
	if q.Version != 1 || !strings.Contains(q.TermsReference, q.ID) || q.TermsReference == "client-controlled-reference" {
		t.Fatal("internal references were not generated", q)
	}
	terms, err := ParseProcurementExitTerms(q.ExitTerms)
	if err != nil || !terms.VoluntaryRefund || terms.Method != "remaining_paid_proportional" || !strings.Contains(terms.TermsReference, q.ID) {
		t.Fatal(terms, err)
	}
	order, err := f.s.CreatePurchaseOrder(f.ctx, nil, f.owner, f.channel, q.ID, "accepted-readable-rule")
	if err != nil {
		t.Fatal(err)
	}
	next := publish("non_refundable")
	if next.Version != 2 {
		t.Fatal("quote versions were not generated", next.Version)
	}
	stored, err := f.s.GetPurchase(f.ctx, nil, f.owner, f.channel, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored.ExitTerms) != string(order.ExitTerms) || stored.Name != q.Name || stored.TermsSummary != q.TermsSummary || stored.QuoteVersion != 1 {
		t.Fatal("new quote changed accepted order", stored)
	}
	again, err := f.s.CreatePurchaseOrder(f.ctx, nil, f.owner, f.channel, q.ID, "accepted-readable-rule")
	if err != nil || again.ID != order.ID {
		t.Fatal("same-key purchase retry duplicated order", again, err)
	}
	if err = f.db.Model(&PurchaseQuote{}).Where("id=?", q.ID).Update("exit_terms", next.ExitTerms).Error; err == nil {
		t.Fatal("accepted quote rules could be overwritten")
	}
	if err = f.db.Model(&PurchaseOrder{}).Where("id=?", order.ID).Update("terms_summary", "changed after acceptance").Error; err == nil {
		t.Fatal("accepted order rules could be overwritten")
	}
	if err = f.s.SetQuoteEnabled(f.ctx, nil, f.owner, q.ID, false); err != nil {
		t.Fatal("immutable quote cannot be disabled", err)
	}
}

func TestPostgresCooperationRejectsForeignAndPathContractReferences(t *testing.T) {
	f := setup(t)
	ownID, foreignID, pendingID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, r := range []struct{ id, owner, status string }{{ownID, f.platform, "ready"}, {foreignID, f.channel, "ready"}, {pendingID, f.platform, "pending"}} {
		err := f.db.Exec(`INSERT INTO organization_cooperation_contracts(id,operator_id,business_key,fingerprint,object_key,filename,content_type,size_bytes,status,created_by,created_at) VALUES(?,?,?,?,?,?,'application/pdf',10,?,?,?)`, r.id, r.owner, uuid.NewString(), "digest", "cooperation-contracts/"+r.owner+"/"+r.id, "signed.pdf", r.status, f.owner, time.Now()).Error
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{foreignID, pendingID, "/private/contracts/signed.pdf", "https://example.test/signed.pdf"} {
		_, err := f.s.PublishPurchaseQuote(f.ctx, nil, f.owner, PurchaseQuote{SupplierID: f.platform, BuyerID: f.channel, PriceUSD: amount("10"), PaidATG: amount("100"), ContractAttachmentID: id, Enabled: true})
		if !errors.Is(err, organization.ErrForbidden) && !errors.Is(err, organization.ErrInvalidInput) {
			t.Fatalf("unowned contract accepted %q: %v", id, err)
		}
		_, err = f.org.CreateOperator(f.ctx, nil, organization.CreateOperatorInput{ActorStaffID: f.owner, ParentOperatorID: f.platform, Kind: organization.Channel, Name: "invalid document buyer", ContractAttachmentID: id})
		if !errors.Is(err, organization.ErrForbidden) && !errors.Is(err, organization.ErrInvalidInput) {
			t.Fatalf("operator linked unowned contract %q: %v", id, err)
		}
	}
	created, err := f.org.CreateOperator(f.ctx, nil, organization.CreateOperatorInput{ActorStaffID: f.owner, ParentOperatorID: f.platform, Kind: organization.OEM, Name: "new partner", BrandName: "Partner", SupplyTerms: "按双方已确认报价采购", ContractAttachmentID: ownID})
	if err != nil {
		t.Fatal(err)
	}
	if created.Supply.ContractAttachmentID != ownID || created.Supply.ExitPolicy != "non_refundable" || created.Supply.Version != 1 || created.Supply.Status != "active" {
		t.Fatal("cooperation did not generate safe defaults", created)
	}
	if err = f.db.Table("organization_cooperation_contracts").Where("id=?", ownID).Update("object_key", "finance-evidence/foreign/stolen").Error; err == nil {
		t.Fatal("contract content could be replaced after acceptance")
	}
	if err = f.db.Exec("DELETE FROM organization_cooperation_contracts WHERE id=?", ownID).Error; err == nil {
		t.Fatal("accepted attachment could be deleted")
	}
}
