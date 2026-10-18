package portal

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

// Financial actions remain browser-driven. These identities carry no opening
// customer balance, receipts, orders or refund results.
func (f *testApp) seedFinanceSimulation(t *testing.T) map[string]any {
	t.Helper()
	ctx := context.Background()
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	staff := func(email string) map[string]any {
		t.Helper()
		password := "simulation-finance-password"
		hash, err := organization.HashPassword(password)
		if err != nil {
			t.Fatal(err)
		}
		row := Staff{ID: uuid.NewString(), Email: email, DisplayName: "财务仿真", PasswordHash: hash, Version: 1, Status: "active", EmailVerified: true, CreatedAt: time.Now()}
		if err = f.a.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		invite, err := f.a.Organization.InviteStaff(ctx, nil, owner, op, email, "finance")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.a.Organization.AcceptStaffInvitation(ctx, nil, invite.Token, organization.VerifiedStaff{ID: row.ID, Email: email, EmailVerified: true}); err != nil {
			t.Fatal(err)
		}
		return map[string]any{"id": row.ID, "email": email, "password": password}
	}
	finance, reviewer := staff("finance@simulation.test"), staff("finance-reviewer@simulation.test")
	email := "finance-customer@simulation.test"
	f.registered(t, email)
	f.login(t, "customer", email, "customer-password-test")
	status, session, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, session)
	return map[string]any{"operator_id": op, "finance": finance, "reviewer": reviewer, "customer": map[string]any{"id": session["customer"].(map[string]any)["id"], "email": email, "password": "customer-password-test"}}
}
