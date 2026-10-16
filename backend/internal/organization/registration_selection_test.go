package organization

import (
	"errors"
	"testing"
)

func TestPostgresExplicitRegistrationChoiceBindsFinalProofAndCannotBeOverriddenByPrefill(t *testing.T) {
	w := world(t)
	a, _ := w.child(t, w.platform, w.platformOwner, Channel, "Prefilled channel")
	b, _ := w.child(t, w.platform, w.platformOwner, Channel, "Explicitly selected channel")
	prefilled := w.register(t, w.platform.Brand.ID, "prefilled-inviter@example.test", a.InviteCode)
	selected := w.register(t, w.platform.Brand.ID, "selected-inviter@example.test", b.InviteCode)
	preview, e := w.s.ResolveRegistrationIntent(w.ctx, nil, w.platform.Brand.ID, prefilled.InviteCode)
	if e != nil || preview.Operator.ID != a.Operator.ID {
		t.Fatal(preview, e)
	}
	old := w.challenge(t, w.platform.Brand.ID, "explicit-choice@example.test", prefilled.InviteCode)
	final := w.challenge(t, w.platform.Brand.ID, "explicit-choice@example.test", selected.InviteCode)
	if _, e = w.s.CompleteRegistration(w.ctx, nil, CompleteRegistrationInput{IntentID: final.Intent.ID, VerificationToken: old.VerificationToken}); !errors.Is(e, ErrInvalidVerification) {
		t.Fatal("old proof applied to explicit new choice", e)
	}
	r, e := w.s.CompleteRegistration(w.ctx, nil, CompleteRegistrationInput{IntentID: final.Intent.ID, VerificationToken: final.VerificationToken})
	if e != nil || r.Ownership.OperatorID != b.Operator.ID || r.InviterID == nil || *r.InviterID != selected.Customer.ID {
		t.Fatal("prefill overrode explicit final choice", r, e)
	}
	if _, e = w.s.CompleteRegistration(w.ctx, nil, CompleteRegistrationInput{IntentID: old.Intent.ID, VerificationToken: old.VerificationToken}); !errors.Is(e, ErrEmailAlreadyRegistered) {
		t.Fatal("stale registration replaced established ownership", e)
	}
	chain, e := w.s.ReferralChain(w.ctx, nil, r.Customer.ID)
	if e != nil || len(chain) != 1 || chain[0].InviterID != selected.Customer.ID {
		t.Fatal(chain, e)
	}
}
