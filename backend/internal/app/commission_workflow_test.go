package app_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
)

func TestCommissionWorkflowPreviewAndActualFacts(t *testing.T) {
	fx := newWMeterEnv(t)
	ctx := context.Background()
	a := fx.app
	admin := a.Config.BootstrapAdmin
	channel := id.New("t07channel")
	if err := a.DB.Exec("INSERT INTO identity_channel_orgs(id,code,type,parent_id,status,brand_id) SELECT ?,?,type,parent_id,status,brand_id FROM identity_channel_orgs WHERE id=?", channel, channel, identity.ResellerChannelID).Error; err != nil {
		t.Fatal(err)
	}
	var receiver struct{ ID string }
	if err := a.DB.Table("identity_users").Where("email=?", "kol2.b@tokenhub.local").First(&receiver).Error; err != nil {
		t.Fatal(err)
	}
	balance := func() *billing.BalanceView {
		row, err := a.Billing.Balance(ctx, receiver.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	baseline := balance()
	accrue := func(due bool) string {
		usage := id.New("t07usage")
		if _, err := a.Commission.Accrue(ctx, commission.AccrueInput{UsageEventID: usage, RequestID: usage, ChannelOrgID: channel, RoleID: identity.KOL2BRoleID, WholesaleMinor: 2_000_000, CanCommission: true}); err != nil {
			t.Fatal(err)
		}
		if due {
			if err := a.Commission.ForceAvailableAt(ctx, usage, time.Now().Add(-time.Hour)); err != nil {
				t.Fatal(err)
			}
			if _, err := a.Commission.UnfreezeUsage(ctx, time.Now(), usage); err != nil {
				t.Fatal(err)
			}
		}
		return usage
	}
	u1, u2 := accrue(true), accrue(true)
	accrue(false)
	previewURL := fx.server.URL + "/admin/commissions/settlement-preview?channel_id=" + url.QueryEscape(channel)
	preview := func(ignore bool) map[string]any {
		target := previewURL
		if ignore {
			target += "&ignore_minimum=1"
		}
		code, body := doJSON(t, http.MethodGet, target, admin, false, nil)
		if code != 200 {
			t.Fatalf("preview %d %+v", code, body)
		}
		return body["preview"].(map[string]any)
	}
	minimum := preview(false)
	if asInt(minimum["entry_count"]) != 0 || asInt(minimum["excluded"].(map[string]any)["below_minimum"].(map[string]any)["entry_count"]) != 2 || asInt(minimum["excluded"].(map[string]any)["frozen"].(map[string]any)["entry_count"]) != 1 {
		t.Fatalf("untruthful preview %+v", minimum)
	}
	original := preview(true)
	if asInt(original["amount_minor"]) != 600000 || asInt(original["entry_count"]) != 2 || asInt(original["recipient_count"]) != 1 {
		t.Fatalf("preview facts %+v", original)
	}
	settleURL := fx.server.URL + "/admin/commissions/settle?channel_id=" + url.QueryEscape(channel)
	old := commission.SettlementCreateInput{OperationID: id.New("operation"), PreviewID: original["id"].(string)}
	accrue(true)
	if code, body := doJSON(t, http.MethodPost, settleURL, admin, true, old); code != 409 {
		t.Fatalf("changed candidate committed %d %+v", code, body)
	}
	current := preview(true)
	in := commission.SettlementCreateInput{OperationID: id.New("operation"), PreviewID: current["id"].(string)}
	responses := make(chan map[string]any, 3)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, body := doJSON(t, http.MethodPost, settleURL, admin, true, in)
			if code != 200 {
				t.Errorf("concurrent settlement %d %+v", code, body)
			}
			responses <- body
		}()
	}
	wg.Wait()
	close(responses)
	sid := ""
	for body := range responses {
		rows := body["items"].([]any)
		if len(rows) != 1 {
			t.Fatalf("settlement result %+v", body)
		}
		row := rows[0].(map[string]any)
		if asInt(row["amount_minor"]) != 900000 {
			t.Fatalf("original amount %+v", row)
		}
		next := row["id"].(string)
		if sid != "" && sid != next {
			t.Fatal("same operation made different settlements")
		}
		sid = next
	}
	changed := in
	changed.PreviewID = original["id"].(string)
	if code, _ := doJSON(t, http.MethodPost, settleURL, admin, true, changed); code != 409 {
		t.Fatalf("changed original payload accepted %d", code)
	}
	operationURL := fx.server.URL + "/admin/commission-operations/" + in.OperationID
	if code, body := doJSON(t, http.MethodGet, operationURL, admin, false, nil); code != 200 || body["item"].(map[string]any)["kind"] != "settle" {
		t.Fatalf("original batch not recoverable %d %+v", code, body)
	}
	payoutURL := fx.server.URL + "/admin/settlements/" + sid + "/payout"
	payout := commission.PayoutInput{AmountMinor: 900000, OperationID: id.New("operation"), Method: "manual", OccurredAt: time.Now().UTC().Add(-time.Minute), Confirmed: true, Note: "现金付款"}
	wrong := payout
	wrong.AmountMinor = 1
	if code, _ := doJSON(t, http.MethodPost, payoutURL, admin, true, wrong); code != 409 {
		t.Fatalf("payout amount changed %d", code)
	}
	// Audit failure must roll back cash, operation, payout and status together.
	callback := "t07_payout_audit_failure"
	if err := a.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "audit_logs" {
			tx.AddError(errors.New("injected audit failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	code, _ := doJSON(t, http.MethodPost, payoutURL, admin, true, payout)
	a.DB.Callback().Create().Remove(callback)
	if code != 500 {
		t.Fatalf("audit failure %d", code)
	}
	if balance().CommissionAvailableMinor != baseline.CommissionAvailableMinor+900000 {
		t.Fatal("failed audit debited commission")
	}
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, body := doJSON(t, http.MethodPost, payoutURL, admin, true, payout)
			if code != 200 || body["item"].(map[string]any)["payout_id"] == "" {
				t.Errorf("empty-reference payout %d %+v", code, body)
			}
		}()
	}
	wg.Wait()
	paid := balance()
	if paid.CommissionAvailableMinor != baseline.CommissionAvailableMinor || paid.AvailableMinor != baseline.AvailableMinor || paid.GiftMinor != baseline.GiftMinor {
		t.Fatal("payout moved wrong funds or duplicated")
	}
	lookup := fx.server.URL + "/admin/commission-operations/" + payout.OperationID
	if code, body := doJSON(t, http.MethodGet, lookup, admin, false, nil); code != 200 || body["item"].(map[string]any)["result"].(map[string]any)["payout_occurred_at"] == nil {
		t.Fatalf("actual payout not recoverable %d %+v", code, body)
	}
	finance := tokenOf(postBody(t, fx.server.URL+"/v1/auth/login", "", map[string]string{"email": "finance@tokenhub.local", "password": "password1"}))
	if code, _ := doJSON(t, http.MethodGet, lookup, finance, false, nil); code != 404 {
		t.Fatalf("another actor read original operation %d", code)
	}
	if code, _ := doJSON(t, http.MethodGet, fx.server.URL+"/channel/commission-operations/"+payout.OperationID, admin+"-c", false, nil); code != 404 {
		t.Fatalf("another brand read original operation %d", code)
	}
	if code, _ := doJSON(t, http.MethodPost, payoutURL, admin+"-audit", true, payout); code != 403 {
		t.Fatalf("readonly payout %d", code)
	}
	if code, _ := doJSON(t, http.MethodGet, fx.server.URL+"/admin/settlements/"+sid, admin+"-audit", false, nil); code != 200 {
		t.Fatalf("readonly detail %d", code)
	}
	for _, usage := range []string{u1, u2} {
		if err := a.Commission.Reverse(ctx, usage); err != nil {
			t.Fatal(err)
		}
	}
	claims, err := a.Billing.ListScopedCommissionRecoveries(ctx, []string{sid})
	if err != nil || len(claims) != 2 {
		t.Fatalf("claims %+v %v", claims, err)
	}
	firstURL := fx.server.URL + "/admin/commission-recoveries/" + claims[0].ID + "/receipts"
	receipt := billing.RecoveryReceiptInput{OccurredAt: payout.OccurredAt, Confirmed: true, AmountMinor: 100000, Note: "现金收回", IdempotencyKey: id.New("operation")}
	for n := 0; n < 2; n++ {
		if code, body := doJSON(t, http.MethodPost, firstURL, admin, true, receipt); code != 200 || body["item"].(map[string]any)["reference"] != "" {
			t.Fatalf("optional proof/replay %d %+v", code, body)
		}
	}
	// Distinct real receipts may share a note without being the same operation.
	second := receipt
	second.IdempotencyKey = id.New("operation")
	if code, _ := doJSON(t, http.MethodPost, firstURL, admin, true, second); code != 200 {
		t.Fatalf("same note rejected %d", code)
	}
	over := receipt
	over.IdempotencyKey = id.New("operation")
	over.AmountMinor = 100001
	if code, _ := doJSON(t, http.MethodPost, firstURL, admin, true, over); code != 400 {
		t.Fatalf("overcollection %d", code)
	}
	proof := receipt
	proof.IdempotencyKey = id.New("operation")
	proof.Reference = "BANK-" + id.New("transaction")
	proof.AmountMinor = 100000
	if code, _ := doJSON(t, http.MethodPost, firstURL, admin, true, proof); code != 200 {
		t.Fatalf("last receipt %d", code)
	}
	proof.IdempotencyKey = id.New("operation")
	secondURL := fx.server.URL + "/admin/commission-recoveries/" + claims[1].ID + "/receipts"
	if code, _ := doJSON(t, http.MethodPost, secondURL, admin, true, proof); code != 409 {
		t.Fatalf("one external transaction counted twice %d", code)
	}
	if code, body := doJSON(t, http.MethodGet, fx.server.URL+"/admin/commission-recovery-operations/"+receipt.IdempotencyKey, admin, false, nil); code != 200 || asInt(body["item"].(map[string]any)["amount_minor"]) != 100000 {
		t.Fatalf("receipt lookup %d %+v", code, body)
	}
	if code, _ := doJSON(t, http.MethodGet, fx.server.URL+"/admin/commission-recovery-operations/"+receipt.IdempotencyKey, finance, false, nil); code != 404 {
		t.Fatalf("cross-actor receipt lookup %d", code)
	}
	if balance().CommissionAvailableMinor != paid.CommissionAvailableMinor || balance().AvailableMinor != paid.AvailableMinor {
		t.Fatal("recovery changed commission/API wallet")
	}
}

func TestCommissionWorkflowCompleteServerSearchAndPagination(t *testing.T) {
	fx := newWMeterEnv(t)
	a := fx.app
	channel := id.New("t07page")
	if err := a.DB.Exec("INSERT INTO identity_channel_orgs(id,code,type,parent_id,status,brand_id) SELECT ?,?,type,parent_id,status,brand_id FROM identity_channel_orgs WHERE id=?", channel, channel, identity.ResellerChannelID).Error; err != nil {
		t.Fatal(err)
	}
	prefix := id.New("page")
	for i := 0; i < 153; i++ {
		if err := a.DB.Exec("INSERT INTO commission_settlements(id,period_start,period_end,channel_org_id,beneficiary_role_id,amount_minor,status,policy_version,created_at) VALUES (?,now(),now(),?,?,100,'settled','fixture',?)", fmt.Sprintf("%s_%03d", prefix, i), channel, identity.KOL2BRoleID, time.Now().UTC().Add(time.Duration(i)*time.Second)).Error; err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		target := fx.server.URL + "/admin/settlements?limit=40&channel_id=" + url.QueryEscape(channel) + "&status=settled&q=kol2.b%40tokenhub.local&cursor=" + url.QueryEscape(cursor)
		code, body := doJSON(t, http.MethodGet, target, a.Config.BootstrapAdmin, false, nil)
		if code != 200 || asInt(body["total"]) != 153 {
			t.Fatalf("full search %d %+v", code, body)
		}
		for _, raw := range body["items"].([]any) {
			sid := raw.(map[string]any)["id"].(string)
			if seen[sid] {
				t.Fatal("duplicate page record")
			}
			seen[sid] = true
		}
		cursor = body["next_cursor"].(string)
		if cursor == "" {
			break
		}
	}
	if len(seen) != 153 {
		t.Fatalf("truncated full catalog: %d", len(seen))
	}
	code, body := doJSON(t, http.MethodGet, fx.server.URL+"/admin/settlements?channel_id="+url.QueryEscape(channel)+"&q="+url.QueryEscape(prefix+"_000"), a.Config.BootstrapAdmin+"-audit", false, nil)
	if code != 200 || len(body["items"].([]any)) != 1 {
		t.Fatalf("oldest record unsearchable %d %+v", code, body)
	}
	if code, _ := doJSON(t, http.MethodGet, fx.server.URL+"/admin/settlements?channel_id="+identity.OEMChannelID, a.Config.BootstrapAdmin, false, nil); code != 403 {
		t.Fatalf("foreign brand scope read %d", code)
	}
}

func TestCommissionWorkflowRejectsExpiredAndChangedPolicyPreview(t *testing.T) {
	fx := newWMeterEnv(t)
	a := fx.app
	ctx := context.Background()
	channel := id.New("t07preview")
	if err := a.DB.Exec("INSERT INTO identity_channel_orgs(id,code,type,parent_id,status,brand_id) SELECT ?,?,type,parent_id,status,brand_id FROM identity_channel_orgs WHERE id=?", channel, channel, identity.ResellerChannelID).Error; err != nil {
		t.Fatal(err)
	}
	usage := id.New("t07usage")
	if _, err := a.Commission.Accrue(ctx, commission.AccrueInput{UsageEventID: usage, RequestID: usage, ChannelOrgID: channel, RoleID: identity.KOL2BRoleID, WholesaleMinor: 2_000_000, CanCommission: true}); err != nil {
		t.Fatal(err)
	}
	if err := a.Commission.ForceAvailableAt(ctx, usage, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Commission.UnfreezeUsage(ctx, time.Now(), usage); err != nil {
		t.Fatal(err)
	}
	get := func() string {
		code, body := doJSON(t, http.MethodGet, fx.server.URL+"/admin/commissions/settlement-preview?ignore_minimum=1&channel_id="+channel, a.Config.BootstrapAdmin, false, nil)
		if code != 200 {
			t.Fatalf("preview %d %+v", code, body)
		}
		return body["preview"].(map[string]any)["id"].(string)
	}
	post := func(preview string) int {
		code, _ := doJSON(t, http.MethodPost, fx.server.URL+"/admin/commissions/settle?channel_id="+channel, a.Config.BootstrapAdmin, true, commission.SettlementCreateInput{OperationID: id.New("operation"), PreviewID: preview})
		return code
	}
	expired := get()
	if err := a.DB.Table("commission_workflow_previews").Where("id=?", expired).Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if code := post(expired); code != 409 {
		t.Fatalf("expired preview committed %d", code)
	}
	old := get()
	policy, err := a.Commission.ActivePolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	changed := *policy
	changed.ExpectedVersion = policy.Version
	changed.FreezeDays++
	if _, err := a.Commission.UpdatePolicy(ctx, changed); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		current, e := a.Commission.ActivePolicy(ctx)
		if e != nil {
			t.Error(e)
			return
		}
		restore := *policy
		restore.ExpectedVersion = current.Version
		if _, e = a.Commission.UpdatePolicy(ctx, restore); e != nil {
			t.Error(e)
		}
	})
	if code := post(old); code != 409 {
		t.Fatalf("changed policy committed %d", code)
	}
	if code := post(get()); code != 200 {
		t.Fatalf("fresh current preview rejected %d", code)
	}
	facts, err := a.Commission.RequestFacts(ctx, usage)
	if err != nil || len(facts) != 1 || facts[0].UsageEventID != usage || facts[0].BeneficiaryRoleID != identity.KOL2BRoleID || facts[0].SettlementID == "" {
		t.Fatalf("request original facts %+v %v", facts, err)
	}
}

func TestCommissionWorkflowChannelOwnFactsOnly(t *testing.T) {
	fx := newWMeterEnv(t)
	a := fx.app
	ctx := context.Background()
	var user struct{ ID string }
	if err := a.DB.Table("identity_users").Where("email=?", "channel.b@tokenhub.local").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	role, err := a.Identity.CreateAcquisitionRole(ctx, identity.ResellerChannelID, identity.AcqPromoter, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Identity.BindRoleMember(ctx, user.ID, role.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := a.DB.Table("identity_role_members").Where("acquisition_role_id=?", role.ID).Delete(nil).Error; e != nil {
			t.Error(e)
		}
	})
	ownUsage, otherUsage := id.New("t07own"), id.New("t07other")
	for usage, recipient := range map[string]string{ownUsage: role.ID, otherUsage: identity.KOL2BRoleID} {
		if _, e := a.Commission.Accrue(ctx, commission.AccrueInput{UsageEventID: usage, ChannelOrgID: identity.ResellerChannelID, RoleID: recipient, WholesaleMinor: 2_000_000, CanCommission: true}); e != nil {
			t.Fatal(e)
		}
	}
	code, body := doJSON(t, http.MethodGet, fx.server.URL+"/channel/commissions?q="+ownUsage, a.Config.BootstrapAdmin+"-b", false, nil)
	if code != 200 || len(body["items"].([]any)) != 1 {
		t.Fatalf("own fact missing %d %+v", code, body)
	}
	code, body = doJSON(t, http.MethodGet, fx.server.URL+"/channel/commissions?q="+otherUsage, a.Config.BootstrapAdmin+"-b", false, nil)
	if code != 200 || len(body["items"].([]any)) != 0 {
		t.Fatalf("another recipient fact exposed %d %+v", code, body)
	}
}

func TestCommissionWorkflowOwnSettlementDeepLink(t *testing.T) {
	fx := newWMeterEnv(t)
	a := fx.app
	ctx := context.Background()
	var user struct{ ID string }
	if err := a.DB.Table("identity_users").Where("email=?", "channel.b@tokenhub.local").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	role, err := a.Identity.CreateAcquisitionRole(ctx, identity.ResellerChannelID, identity.AcqPromoter, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Identity.BindRoleMember(ctx, user.ID, role.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.DB.Table("identity_role_members").Where("acquisition_role_id=?", role.ID).Delete(nil) })
	prefix := id.New("ownsettlement")
	for n := 0; n < 153; n++ {
		if err := a.DB.Exec("INSERT INTO commission_settlements(id,period_start,period_end,channel_org_id,beneficiary_role_id,amount_minor,status,policy_version,created_at) VALUES (?,now(),now(),?,?,100,'settled','original-policy',?)", fmt.Sprintf("%s_%03d", prefix, n), identity.ResellerChannelID, role.ID, time.Now().UTC().Add(time.Duration(n)*time.Second)).Error; err != nil {
			t.Fatal(err)
		}
	}
	original := prefix + "_000"
	code, body := doJSON(t, http.MethodGet, fx.server.URL+"/channel/settlements?limit=30&q="+prefix, a.Config.BootstrapAdmin+"-b", false, nil)
	if code != 200 || asInt(body["total"]) != 153 || len(body["items"].([]any)) != 30 {
		t.Fatalf("own full history %d %+v", code, body)
	}
	for _, raw := range body["items"].([]any) {
		if raw.(map[string]any)["id"] == original {
			t.Fatal("fixture original unexpectedly in first page")
		}
	}
	code, body = doJSON(t, http.MethodGet, fx.server.URL+"/channel/settlements/"+original, a.Config.BootstrapAdmin+"-b", false, nil)
	if code != 200 || body["item"].(map[string]any)["id"] != original || body["entries"] == nil {
		t.Fatalf("own original detail unavailable %d %+v", code, body)
	}
	code, body = doJSON(t, http.MethodGet, fx.server.URL+"/channel/settlements?settlement_id="+original, a.Config.BootstrapAdmin+"-b", false, nil)
	if code != 200 || asInt(body["total"]) != 1 || body["items"].([]any)[0].(map[string]any)["id"] != original {
		t.Fatalf("exact original not consumed %d %+v", code, body)
	}
	foreign := id.New("foreignsettlement")
	if err := a.DB.Exec("INSERT INTO commission_settlements(id,period_start,period_end,channel_org_id,beneficiary_role_id,amount_minor,status,policy_version) VALUES (?,now(),now(),?,?,100,'settled','fixture')", foreign, identity.ResellerChannelID, identity.KOL2BRoleID).Error; err != nil {
		t.Fatal(err)
	}
	if code, _ := doJSON(t, http.MethodGet, fx.server.URL+"/channel/settlements/"+foreign, a.Config.BootstrapAdmin+"-b", false, nil); code != 404 {
		t.Fatalf("other recipient exposed %d", code)
	}
	if code, _ := doJSON(t, http.MethodPost, fx.server.URL+"/channel/settlements/"+original+"/payout", a.Config.BootstrapAdmin+"-b", true, commission.PayoutInput{AmountMinor: 100, OperationID: id.New("op"), Method: "manual", OccurredAt: time.Now().Add(-time.Minute), Confirmed: true}); code != 403 {
		t.Fatalf("B mutated own settlement %d", code)
	}
}
