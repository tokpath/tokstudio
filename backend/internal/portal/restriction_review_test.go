package portal

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
	"gorm.io/gorm"
)

func reviewStaff(t *testing.T, f *testApp, actor, operator, email, role string) Staff {
	t.Helper()
	person := Staff{ID: uuid.NewString(), Email: email, DisplayName: "Review staff", PasswordHash: "synthetic-unusable-login-hash", Version: 1, Status: "active", EmailVerified: true, CreatedAt: time.Now()}
	if e := f.a.DB.Create(&person).Error; e != nil {
		t.Fatal(e)
	}
	invitation, e := f.a.Organization.InviteStaff(context.Background(), nil, actor, operator, email, role)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.a.Organization.AcceptStaffInvitation(context.Background(), nil, invitation.Token, organization.VerifiedStaff{ID: person.ID, Email: email, EmailVerified: true}); e != nil {
		t.Fatal(e)
	}
	return person
}

func reviewChannel(t *testing.T, f *testApp) (organization.OperatorCreated, Staff, Staff) {
	t.Helper()
	ctx := context.Background()
	channel, e := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: f.a.Installation.OwnerStaffID, ParentOperatorID: f.a.Installation.OperatorID, Kind: organization.Channel, Name: "Independent review channel", LegalName: "Synthetic review entity"})
	if e != nil {
		t.Fatal(e)
	}
	owner := reviewStaff(t, f, f.a.Installation.OwnerStaffID, channel.Operator.ID, "channel-owner@review.test", "owner")
	reviewer := reviewStaff(t, f, owner.ID, channel.Operator.ID, "reviewer@review.test", "support")
	if e = f.a.Organization.SetMembership(ctx, nil, organization.SetMembershipInput{ActorStaffID: owner.ID, OperatorID: channel.Operator.ID, StaffID: reviewer.ID, Role: "custom", Status: "active", Grants: []string{"security.manage"}}); e != nil {
		t.Fatal(e)
	}
	return channel, owner, reviewer
}

func reviewCustomer(t *testing.T, f *testApp, actor, operator, code, email string) string {
	t.Helper()
	ctx := context.Background()
	if _, e := f.a.Incentives.SetPolicy(ctx, nil, actor, operator, incentives.PolicyInput{ThresholdMoney: ledger.MustDecimal("1"), DirectRate: ledger.MustDecimal("0.1"), IndirectRate: ledger.MustDecimal("0.02"), RewardValiditySeconds: 86400}); e != nil {
		t.Fatal(e)
	}
	state, e := f.a.Organization.GetBusinessStatus(ctx, nil, operator)
	if e != nil {
		t.Fatal(e)
	}
	if !state.Registration {
		if _, e = f.a.Organization.SetBusinessStatus(ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: actor, OperatorID: operator, Feature: organization.Registration, Enabled: true, ExpectedVersion: state.Version, EvidenceReference: "isolated review recipient scope"}, f.a); e != nil {
			t.Fatal(e)
		}
	}
	challenge, e := f.a.Organization.BeginRegistration(ctx, nil, organization.RegistrationIntentRequest{BrandID: f.a.Installation.BrandID, InviteCode: code, Email: email, Password: "review-customer-password"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.a.Organization.VerifyRegistration(ctx, nil, challenge.Intent.ID, challenge.VerificationToken); e != nil {
		t.Fatal(e)
	}
	customer, e := f.a.Organization.CompleteRegistration(ctx, nil, organization.CompleteRegistrationInput{IntentID: challenge.Intent.ID, VerificationToken: challenge.VerificationToken, DisplayName: "Synthetic review customer"})
	if e != nil {
		t.Fatal(e)
	}
	return customer.Customer.ID
}

// Create each valid, future-dated decision through the actual domain service.
// Moving only its timestamp simulates elapsed time, never a worker result.
func reviewDecision(t *testing.T, f *testApp, actor, operator, kind, id, action string, dueIn time.Duration) organization.ScopeFreezeInput {
	t.Helper()
	future := time.Now().Add(48 * time.Hour)
	in := organization.ScopeFreezeInput{ActorStaffID: actor, OperatorID: operator, ScopeKind: kind, ScopeID: id, Action: action, Enabled: true, Evidence: "synthetic review evidence", Explanation: "limited review with explicit deadline", ReviewDueAt: &future}
	if _, e := f.a.Organization.SetScopeFreeze(context.Background(), nil, in); e != nil {
		t.Fatal(e)
	}
	if e := f.a.DB.Model(&organization.ScopeFreeze{}).Where("scope_kind=? AND scope_id=? AND action=?", kind, id, action).Update("review_due_at", gorm.Expr("clock_timestamp() + (? * interval '1 second')", int64(dueIn/time.Second))).Error; e != nil {
		t.Fatal(e)
	}
	in.ExpectedVersion = 1
	return in
}

func reviewNotices(t *testing.T, f *testApp, operator string) []operations.Notification {
	t.Helper()
	var notices []operations.Notification
	if e := f.a.DB.Where("operator_id=? AND business_key LIKE 'restriction-review:%'", operator).Order("created_at,id").Find(&notices).Error; e != nil {
		t.Fatal(e)
	}
	return notices
}

func TestPostgresRestrictionReviewDueEscalationExactScopeAndLatestMailOnly(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	platform, platformOwner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	channel, owner, reviewer := reviewChannel(t, f)
	finance := reviewStaff(t, f, owner.ID, channel.Operator.ID, "finance@review.test", "finance")
	childCustomer := reviewCustomer(t, f, owner.ID, channel.Operator.ID, channel.InviteCode, "channel-customer@review.test")
	parentCustomer := reviewCustomer(t, f, platformOwner, platform, "", "parent-customer@review.test")
	current := reviewDecision(t, f, reviewer.ID, channel.Operator.ID, "customer", childCustomer, "model_usage", -time.Hour)
	reviewDecision(t, f, owner.ID, channel.Operator.ID, "operator", channel.Operator.ID, "activity", time.Hour)
	released := reviewDecision(t, f, owner.ID, channel.Operator.ID, "customer", childCustomer, "withdrawal", -48*time.Hour)
	released.Enabled = false
	if _, e := f.a.Organization.SetScopeFreeze(ctx, nil, released); e != nil {
		t.Fatal(e)
	}
	reviewDecision(t, f, platformOwner, platform, "brand", f.a.Installation.BrandID, "registration", -26*time.Hour)
	reviewDecision(t, f, platformOwner, platform, "operator", platform, "renewal", -time.Hour)
	reviewDecision(t, f, platformOwner, platform, "customer", parentCustomer, "new_purchase", -time.Hour)
	states, e := f.a.Operations.RunRestrictionReviews(ctx, channel.Operator.ID)
	if e != nil || len(states) != 1 || states[0].Status != "warning" || states[0].ScopeID != childCustomer {
		t.Fatal("deadline or child responsibility mismatch", states, e)
	}
	states, e = f.a.Operations.RunRestrictionReviews(ctx, platform)
	if e != nil || len(states) != 3 {
		t.Fatal("brand/operator/customer scope mismatch", states, e)
	}
	for _, n := range reviewNotices(t, f, platform) {
		if n.RecipientID != platformOwner || strings.Contains(n.Body, childCustomer) {
			t.Fatal("supplier saw child review", n)
		}
	}
	childNotices := reviewNotices(t, f, channel.Operator.ID)
	if len(childNotices) != 1 || childNotices[0].RecipientID != reviewer.ID {
		t.Fatal("warning must reach original authorized reviewer only", childNotices)
	}
	var wg sync.WaitGroup
	errorsCh := make(chan error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := f.a.Operations.RunRestrictionReviews(ctx, channel.Operator.ID)
			errorsCh <- e
		}()
	}
	wg.Wait()
	close(errorsCh)
	for e := range errorsCh {
		if e != nil {
			t.Fatal(e)
		}
	}
	if len(reviewNotices(t, f, channel.Operator.ID)) != 1 {
		t.Fatal("concurrent job replay duplicated warning")
	}
	if e := f.a.DB.Model(&organization.ScopeFreeze{}).Where("scope_kind='customer' AND scope_id=? AND action='model_usage'", childCustomer).Update("review_due_at", gorm.Expr("clock_timestamp()-interval '25 hours'")).Error; e != nil {
		t.Fatal(e)
	}
	states, e = f.a.Operations.RunRestrictionReviews(ctx, channel.Operator.ID)
	if e != nil || len(states) != 1 || states[0].Status != "critical" || states[0].Sequence != 2 {
		t.Fatal("overdue review did not escalate immediately", states, e)
	}
	if len(reviewNotices(t, f, channel.Operator.ID)) != 3 {
		t.Fatal("critical escalation must add the local owner exactly once")
	}
	if _, e = f.a.Operations.RunRestrictionReviews(ctx, channel.Operator.ID); e != nil || len(reviewNotices(t, f, channel.Operator.ID)) != 3 {
		t.Fatal("escalation replay duplicated notices", e)
	}
	if e = f.a.DB.Model(&operations.RestrictionReview{}).Where("operator_id=?", channel.Operator.ID).Update("last_notified_at", gorm.Expr("clock_timestamp()-interval '61 minutes'")).Error; e != nil {
		t.Fatal(e)
	}
	states, e = f.a.Operations.RunRestrictionReviews(ctx, channel.Operator.ID)
	if e != nil || states[0].Sequence != 3 {
		t.Fatal("hourly reminder missing", states, e)
	}
	deliveries, e := f.a.Operations.RunNotifications(ctx, 100)
	if e != nil || len(deliveries) != 8 {
		t.Fatal(deliveries, e)
	}
	accepted, suppressed := 0, 0
	for _, n := range reviewNotices(t, f, channel.Operator.ID) {
		if n.RecipientID == finance.ID || n.RecipientID == platformOwner {
			t.Fatal("review escaped current security authority", n)
		}
		d, e := f.a.Operations.GetDelivery(ctx, nil, n.ID)
		if e != nil {
			t.Fatal(e)
		}
		if strings.HasSuffix(n.BusinessKey, ":3") {
			if d.State != "accepted" || n.Severity != "critical" {
				t.Fatal("current escalation not submitted", n, d)
			}
			accepted++
		} else {
			if d.State != "suppressed" || d.ErrorCode != "review_resolved_or_access_changed" {
				t.Fatal("SMTP recovery submitted superseded hourly warning", n, d)
			}
			suppressed++
		}
	}
	if accepted != 2 || suppressed != 3 {
		t.Fatal(accepted, suppressed)
	}
	if e = f.a.Organization.RequireUnfrozen(ctx, nil, channel.Operator.BrandID, channel.Operator.ID, childCustomer, "model_usage"); !errors.Is(e, organization.ErrFrozen) {
		t.Fatal("overdue reminder silently released restriction", e)
	}
	if e = f.a.Organization.RequireUnfrozen(ctx, nil, channel.Operator.BrandID, channel.Operator.ID, childCustomer, "settlement"); e != nil {
		t.Fatal("review notification changed settlement eligibility", e)
	}
	current.Enabled = false
	if _, e = f.a.Organization.SetScopeFreeze(ctx, nil, current); e != nil {
		t.Fatal(e)
	}
	states, e = f.a.Operations.RunRestrictionReviews(ctx, channel.Operator.ID)
	if e != nil || len(states) != 0 || len(reviewNotices(t, f, channel.Operator.ID)) != 5 {
		t.Fatal("released decision kept notifying", states, e)
	}
	if ds, e := f.a.Operations.RunNotifications(ctx, 100); e != nil || len(ds) != 0 {
		t.Fatal("accepted mail submitted twice", ds, e)
	}
	t.Log("real PostgreSQL: due warning, daily critical, hourly reminder, unchanged restrictions, exact brand/operator/customer scopes and five concurrent replays; SMTP recovery submitted only the latest two local critical emails")
}

func TestPostgresRestrictionReviewQueuedReleaseRescheduleRevocationAndTakeover(t *testing.T) {
	for _, change := range []string{"release", "reschedule", "permission_revoked", "takeover"} {
		t.Run(change, func(t *testing.T) {
			f := setupPortal(t)
			ctx := context.Background()
			channel, owner, reviewer := reviewChannel(t, f)
			customer := reviewCustomer(t, f, owner.ID, channel.Operator.ID, channel.InviteCode, "scope-change@review.test")
			decision := reviewDecision(t, f, reviewer.ID, channel.Operator.ID, "customer", customer, "model_usage", -time.Hour)
			if _, e := f.a.Operations.RunRestrictionReviews(ctx, channel.Operator.ID); e != nil {
				t.Fatal(e)
			}
			switch change {
			case "release", "reschedule":
				decision.Enabled = change != "release"
				if _, e := f.a.Organization.SetScopeFreeze(ctx, nil, decision); e != nil {
					t.Fatal(e)
				}
			case "permission_revoked":
				if e := f.a.Organization.SetMembership(ctx, nil, organization.SetMembershipInput{ActorStaffID: owner.ID, OperatorID: channel.Operator.ID, StaffID: reviewer.ID, Role: "support", Status: "active"}); e != nil {
					t.Fatal(e)
				}
			case "takeover":
				if e := f.a.DB.Transaction(func(tx *gorm.DB) error {
					_, e := f.a.Organization.ApplyChannelTakeoverOwnership(ctx, tx, organization.TakeoverOwnershipInput{ChannelID: channel.Operator.ID, BrandOwnerOperatorID: f.a.Installation.OperatorID, EvidenceReference: "synthetic approved ownership transfer for review scope", ExpectedVersions: map[string]int64{customer: 1}})
					return e
				}); e != nil {
					t.Fatal(e)
				}
			}
			ds, e := f.a.Operations.RunNotifications(ctx, 10)
			if e != nil || len(ds) != 1 || ds[0].State != "suppressed" {
				t.Fatal("queued stale decision submitted after scope/version/access changed", ds, e)
			}
			if len(f.box.messages) != 0 {
				t.Fatal("obsolete review email reached sender")
			}
			if change == "takeover" {
				states, e := f.a.Operations.RunRestrictionReviews(ctx, channel.Operator.ID)
				if e != nil || len(states) != 0 {
					t.Fatal("former channel retained customer review", states, e)
				}
				states, e = f.a.Operations.RunRestrictionReviews(ctx, f.a.Installation.OperatorID)
				if e != nil || len(states) != 1 || states[0].ScopeID != customer {
					t.Fatal("current responsibility lost overdue review", states, e)
				}
				current := reviewNotices(t, f, f.a.Installation.OperatorID)
				if len(current) != 1 || current[0].RecipientID != f.a.Installation.OwnerStaffID {
					t.Fatal("new owner did not assume review", current)
				}
			}
			if change == "permission_revoked" {
				if e = f.a.DB.Model(&operations.RestrictionReview{}).Where("operator_id=?", channel.Operator.ID).Update("last_notified_at", gorm.Expr("clock_timestamp()-interval '61 minutes'")).Error; e != nil {
					t.Fatal(e)
				}
				if _, e = f.a.Operations.RunRestrictionReviews(ctx, channel.Operator.ID); e != nil {
					t.Fatal(e)
				}
				notices := reviewNotices(t, f, channel.Operator.ID)
				if len(notices) != 2 || notices[1].RecipientID != owner.ID {
					t.Fatal("revoked review did not fall back to local owner", notices)
				}
			}
		})
	}
}

type reviewRetrySender struct {
	fail bool
	ids  []string
}

func (s *reviewRetrySender) Send(_ context.Context, m mail.Message) error {
	s.ids = append(s.ids, m.ID)
	if s.fail {
		return errors.New("synthetic SMTP temporary failure")
	}
	return nil
}

func TestPostgresRestrictionReviewSubmissionRetriesOriginalNotification(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	op := f.a.Installation.OperatorID
	reviewDecision(t, f, f.a.Installation.OwnerStaffID, op, "operator", op, "model_usage", -time.Hour)
	sender := &reviewRetrySender{fail: true}
	f.a.Mail.SetSender(sender)
	if _, e := f.a.Operations.RunRestrictionReviews(ctx, op); e != nil {
		t.Fatal(e)
	}
	n := reviewNotices(t, f, op)[0]
	ds, e := f.a.Operations.RunNotifications(ctx, 10)
	if e != nil || len(ds) != 1 || ds[0].State != "pending" || ds[0].Attempts != 1 {
		t.Fatal(ds, e)
	}
	if _, e = f.a.Operations.RunRestrictionReviews(ctx, op); e != nil || len(reviewNotices(t, f, op)) != 1 {
		t.Fatal("maintenance retry created a new mail intent", e)
	}
	if ds, e = f.a.Operations.RunNotifications(ctx, 10); e != nil || len(ds) != 0 {
		t.Fatal("mail backoff bypassed", ds, e)
	}
	sender.fail = false
	if e = f.a.DB.Table("operations_deliveries").Where("notification_id=?", n.ID).Update("next_attempt_at", gorm.Expr("clock_timestamp()-interval '1 second'")).Error; e != nil {
		t.Fatal(e)
	}
	ds, e = f.a.Operations.RunNotifications(ctx, 10)
	if e != nil || len(ds) != 1 || ds[0].State != "accepted" || ds[0].Attempts != 2 || ds[0].NotificationID != n.ID {
		t.Fatal("submission retry did not keep the original durable identity", ds, e)
	}
	if len(sender.ids) != 2 || sender.ids[0] != n.ID || sender.ids[1] != n.ID {
		t.Fatal("Message-ID changed across retry", sender.ids)
	}
	if ds, e = f.a.Operations.RunNotifications(ctx, 10); e != nil || len(ds) != 0 {
		t.Fatal("accepted mail retried", ds, e)
	}
}
