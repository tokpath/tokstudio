package portal

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestPostgresMonetaryOwnerScheduleCancelAndPermanentLock(t *testing.T) {
	f := setupPortalMonetary(t, false)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, out, _ := f.call(t, "GET", "/v1/monetary/settings", nil, "")
	mustOK(t, status, out)
	if out["status"] != "unconfigured" {
		t.Fatal(out)
	}
	status, out, _ = f.call(t, "POST", "/v1/operators/"+f.a.Installation.OperatorID+"/business-status", map[string]any{"feature": "registration", "enabled": true, "expected_version": 1}, "")
	if status != 409 || out["error"].(map[string]any)["code"] != "monetary_not_ready" {
		t.Fatal(status, out)
	}
	// An ordinary role with a broad custom grant still cannot change the unit.
	other := uuid.NewString()
	if e := f.a.requireMonetaryOwner(context.Background(), f.a.DB, other); !errors.Is(e, gorm.ErrRecordNotFound) && e == nil {
		t.Fatal("unknown user became platform owner")
	}
	at := time.Now().UTC().Add(time.Minute)
	in := map[string]any{"currency": "CNY", "atg_per_unit": "10", "effective_at": at, "expected_version": 1, "confirmed": false}
	status, out, _ = f.call(t, "POST", "/v1/monetary/settings", in, "")
	if status != 400 {
		t.Fatal("missing risk acknowledgement accepted", status, out)
	}
	in["confirmed"] = true
	key := uuid.NewString()
	status, out, _ = f.call(t, "POST", "/v1/monetary/settings", in, key)
	mustOK(t, status, out)
	status, replay, _ := f.call(t, "POST", "/v1/monetary/settings", in, key)
	mustOK(t, status, replay)
	if out["version"] != replay["version"] {
		t.Fatal("replay scheduled twice", out, replay)
	}
	status, out, _ = f.call(t, "POST", "/v1/monetary/cancel", map[string]any{"expected_version": 1}, "")
	if status != 409 {
		t.Fatal("stale cancellation accepted", status, out)
	}
	status, out, _ = f.call(t, "POST", "/v1/monetary/cancel", map[string]any{"expected_version": 2}, "")
	mustOK(t, status, out)
	at = time.Now().UTC().Add(time.Second)
	in["expected_version"], in["effective_at"] = 3, at
	status, out, _ = f.call(t, "POST", "/v1/monetary/settings", in, "")
	mustOK(t, status, out)
	time.Sleep(time.Until(at.Add(25 * time.Millisecond)))
	// The database owns the effective-time clock. A container clock can lag
	// the test host slightly; wait for the transition instead of assuming 25ms.
	deadline := time.Now().Add(3 * time.Second)
	for {
		status, out, _ = f.call(t, "GET", "/v1/monetary/settings", nil, "")
		mustOK(t, status, out)
		if out["status"] != "scheduled" || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if out["status"] != "locked" || out["currency"] != "CNY" || out["atg_per_unit"] != "10" {
		t.Fatal(out)
	}
	status, out, _ = f.call(t, "POST", "/v1/monetary/cancel", map[string]any{"expected_version": 4}, "")
	if status != 409 || out["error"].(map[string]any)["code"] != "monetary_locked" {
		t.Fatal("effective foundation unlocked", status, out)
	}
	if e := f.a.DB.Exec("UPDATE ledger_monetary_settings SET currency='EUR'").Error; e == nil {
		t.Fatal("database allowed locked currency mutation")
	}
	if e := f.a.DB.Exec("DELETE FROM ledger_monetary_settings").Error; e == nil {
		t.Fatal("database allowed settings deletion")
	}
	var count int64
	if e := f.a.DB.Table("ledger_monetary_events").Count(&count).Error; e != nil || count != 3 {
		t.Fatal("audit missing or duplicate", count, e)
	}
	row, e := ledger.GetMonetary(context.Background(), f.a.DB)
	if e != nil || row.Currency != "CNY" {
		t.Fatal(row, e)
	}
}

func TestPostgresMonetaryRejectsDelegatedAndChildOwners(t *testing.T) {
	f := setupPortalMonetary(t, false)
	ctx := context.Background()
	platform, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	finance := reviewStaff(t, f, owner, platform, "monetary-finance@example.test", "finance")
	if e := f.a.Organization.SetMembership(ctx, nil, organization.SetMembershipInput{ActorStaffID: owner, OperatorID: platform, StaffID: finance.ID, Role: "custom", Status: "active", Grants: []string{"business.manage", "platform.manage"}}); e != nil {
		t.Fatal(e)
	}
	channel, childOwner, _ := reviewChannel(t, f)
	if channel.Operator.ID == platform {
		t.Fatal("invalid fixture")
	}
	for _, staff := range []Staff{finance, childOwner} {
		if e := f.a.requireMonetaryOwner(ctx, f.a.DB, staff.ID); !errors.Is(e, organization.ErrForbidden) {
			t.Fatal("delegated permission or child owner changed foundation", e)
		}
	}
	hash, e := organization.HashPassword("monetary-fixture-password")
	if e != nil {
		t.Fatal(e)
	}
	if e = f.a.DB.Model(&Staff{}).Where("id=?", finance.ID).Update("password_hash", hash).Error; e != nil {
		t.Fatal(e)
	}
	f.login(t, "staff", finance.Email, "monetary-fixture-password")
	status, out, _ := f.call(t, "POST", "/v1/monetary/settings", map[string]any{"currency": "CNY", "atg_per_unit": "10", "effective_at": time.Now().Add(time.Hour), "confirmed": true, "expected_version": 1}, "")
	if status != 403 {
		t.Fatal("HTTP accepted non-owner", status, out)
	}
	row, e := ledger.GetMonetary(ctx, f.a.DB)
	if e != nil || row.Version != 1 || row.Status != "unconfigured" {
		t.Fatal("forbidden request mutated settings", row, e)
	}
}

func TestPostgresCurrencySchemaKeepsOriginalProviderEvidence(t *testing.T) {
	f := setupPortalMonetary(t, false)
	var value string
	e := f.a.DB.Raw(`SELECT ledger_neutral_money_json('{"paid_usd":"70","evidence":{"provider_fee_usd":"0.2","currency":"USD"},"sources":[{"price_usd":"2"}]}'::jsonb)::text`).Scan(&value).Error
	if e != nil {
		t.Fatal(e)
	}
	var decoded map[string]any
	if e = json.Unmarshal([]byte(value), &decoded); e != nil {
		t.Fatal(e)
	}
	if decoded["paid_money"] != "70" || decoded["evidence"].(map[string]any)["provider_fee_usd"] != "0.2" || decoded["sources"].([]any)[0].(map[string]any)["price_money"] != "2" {
		t.Fatal("original external evidence changed", value)
	}
}
