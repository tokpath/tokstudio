package portal

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestPostgresCoverageWorkspaceParentScopeAndPolicyCAS(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	platform, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	create := func(parent string, kind organization.OperatorKind, name string) organization.Operator {
		t.Helper()
		created, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: parent, Kind: kind, Name: name, LegalName: name + " legal", BrandName: name + " brand"})
		if err != nil {
			t.Fatal(err)
		}
		invite, err := f.a.Organization.InviteStaff(ctx, nil, owner, created.Operator.ID, "owner@example.test", "owner")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.a.Organization.AcceptStaffInvitation(ctx, nil, invite.Token, organization.VerifiedStaff{ID: owner, Email: "owner@example.test", EmailVerified: true}); err != nil {
			t.Fatal(err)
		}
		return created.Operator
	}
	channel := create(platform, organization.Channel, "official channel")
	oem := create(platform, organization.OEM, "OEM A")
	otherOEM := create(platform, organization.OEM, "OEM B")
	oemChannel := create(oem.ID, organization.Channel, "OEM A channel")
	foreignChannel := create(otherOEM.ID, organization.Channel, "OEM B channel")
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	path := func(operator string) string { return "/v1/operators/" + operator + "/coverage" }
	read := func(operator string) map[string]map[string]any {
		t.Helper()
		status, out, _ := f.call(t, "GET", path(operator), nil, "")
		mustOK(t, status, out)
		if len(out["operations"].([]any)) != 0 {
			t.Fatal("obsolete self-only top operation remains", out)
		}
		operations := out["row_operations"].([]any)
		if len(operations) != 1 || operations[0].(map[string]any)["path"] != "/api"+path(operator)+"?target_operator_id={id}" {
			t.Fatal("missing exact target operation", operations)
		}
		rows := map[string]map[string]any{}
		for _, item := range out["items"].([]any) {
			row := item.(map[string]any)
			rows[row["id"].(string)] = row
		}
		return rows
	}
	canEdit := func(row map[string]any) bool {
		for _, action := range row["allowed_actions"].([]any) {
			if action == "oversell" {
				return true
			}
		}
		return false
	}
	post := func(operator, target, allowance string, version int, key string) (int, map[string]any) {
		t.Helper()
		if key != "" {
			key = uuid.NewSHA1(uuid.NameSpaceURL, []byte(key)).String()
		}
		status, out, _ := f.call(t, "POST", path(operator)+"?target_operator_id="+target, map[string]any{"allowance": allowance, "warning": "2.000000000000000001", "expected_version": version}, key)
		return status, out
	}
	rows := read(platform)
	if len(rows) != 4 || rows[oemChannel.ID] != nil || rows[foreignChannel.ID] != nil {
		t.Fatal("platform risk view exceeded direct relationship", rows)
	}
	if canEdit(rows[platform]) || canEdit(rows[oem.ID]) || !canEdit(rows[channel.ID]) || rows[channel.ID]["version"] != float64(0) {
		t.Fatal("incorrect platform/OEM/channel action scope or initial policy version", rows)
	}
	if rows[platform]["values"].(map[string]any)["space"] != nil {
		t.Fatal("platform without upstream was assigned procurement sales space")
	}
	status, out := post(platform, channel.ID, "10.000000000000000001", 0, "initial-parent-channel")
	mustOK(t, status, out)
	if out["id"] != channel.ID || out["version"] != float64(1) {
		t.Fatal("first policy did not retain its target/version", out)
	}
	status, out = post(platform, channel.ID, "10.000000000000000001", 0, "initial-parent-channel")
	mustOK(t, status, out)
	if out["version"] != float64(1) {
		t.Fatal("idempotent retry wrote another policy version", out)
	}
	rows = read(platform)
	if rows[channel.ID]["version"] != float64(1) || rows[channel.ID]["values"].(map[string]any)["oversell"] != "10.000000000000000001" {
		t.Fatal("read projection lost actual version or decimal precision", rows[channel.ID])
	}
	status, out = post(platform, channel.ID, "11", 1, "second-parent-channel")
	mustOK(t, status, out)
	status, out = post(platform, channel.ID, "99", 1, "stale-parent-channel")
	if status != 409 || out["error"].(map[string]any)["code"] != "version_conflict" || out["error"].(map[string]any)["retryable"] != false {
		t.Fatal("stale coverage policy was not a reviewable CAS conflict", status, out)
	}
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			status, _ := post(platform, channel.ID, fmt.Sprint(20+i), 2, fmt.Sprintf("concurrent-parent-channel-%d", i))
			statuses <- status
		}(i)
	}
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatal("concurrent policy updates were not serialized by the real version", counts)
	}
	for _, target := range []string{platform, oem.ID, otherOEM.ID, oemChannel.ID, foreignChannel.ID} {
		status, out = post(platform, target, "100", 0, "")
		if status != 403 {
			t.Fatal("platform changed self, an OEM or a non-direct channel", target, status, out)
		}
	}
	rows = read(oem.ID)
	if len(rows) != 2 || rows[foreignChannel.ID] != nil || !canEdit(rows[oem.ID]) || !canEdit(rows[oemChannel.ID]) {
		t.Fatal("OEM risk scope or independent oversell authority is wrong", rows)
	}
	status, out = post(oem.ID, oem.ID, "8.000000000000000001", 0, "oem-self")
	mustOK(t, status, out)
	status, out = post(oem.ID, oemChannel.ID, "3", 0, "oem-direct-channel")
	mustOK(t, status, out)
	status, out = post(oem.ID, foreignChannel.ID, "4", 0, "oem-foreign-channel")
	if status != 403 {
		t.Fatal("OEM changed another parent's channel", status, out)
	}
	rows = read(channel.ID)
	if len(rows) != 1 || canEdit(rows[channel.ID]) || rows[channel.ID]["version"] != float64(3) {
		t.Fatal("channel self view was writable or lost true version", rows)
	}
	status, out = post(channel.ID, channel.ID, "999", 3, "channel-self")
	if status != 403 {
		t.Fatal("channel changed its own oversell authorization", status, out)
	}
	var policy cooperation.OversellPolicy
	if err := f.a.DB.Where("operator_id=?", channel.ID).Take(&policy).Error; err != nil {
		t.Fatal(err)
	}
	if policy.Version != 3 || policy.Allowance.String() == "999" || policy.WarningBuffer.String() != "2.000000000000000001" {
		t.Fatal("rejected mutations changed the stored authorization", policy)
	}
	// Existing rights without complete model configuration retain a provisional
	// reservation; they cannot turn into zero responsibility or hide siblings.
	_, err := f.a.Ledger.IssueGrant(ctx, nil, ledger.GrantInput{OperatorID: channel.ID, CustomerID: uuid.NewString(), OrderID: uuid.NewString(), BusinessKey: "coverage-price-not-ready", ActorID: owner, Reason: "existing service responsibility", PaidUSD: ledger.MustDecimal("1"), Sources: []ledger.GrantSource{{Kind: "paid", Amount: ledger.MustDecimal("5")}}, EffectiveAt: time.Now().Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	rows = read(platform)
	unknown := rows[channel.ID]
	if len(rows) != 4 || unknown["version"] != float64(3) || unknown["values"].(map[string]any)["required"] != "5" || unknown["values"].(map[string]any)["provisional"] != true || unknown["values"].(map[string]any)["coverage_reason"] == nil {
		t.Fatal("unpriced responsibility was treated as zero or swallowed the risk page", rows)
	}
}
