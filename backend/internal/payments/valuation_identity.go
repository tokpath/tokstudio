package payments

import "github.com/google/uuid"

// EnsureValuationIdentity creates an internal reference for a new valuation.
// The scope must identify the existing receipt or refund execution, not a fresh
// request key. Retries of that same external fact therefore use the same UUID;
// the domain still fingerprints every supplied rate, time window and evidence.
func EnsureValuationIdentity(v Valuation, scope, currency string) Valuation {
	if v.ID == "" {
		v.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("tokstudio-valuation:"+scope)).String()
	}
	if v.Currency == "" {
		v.Currency = currency
	}
	return v
}
