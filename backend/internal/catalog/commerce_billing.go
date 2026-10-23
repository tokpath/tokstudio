package catalog

import (
	"context"
	"strings"

	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

func activePrice(price ResolvedPrice, keys map[string]string) ResolvedPrice {
	out := price
	out.Components = nil
	for _, component := range price.Components {
		if _, ok := keys[component.Key]; ok {
			out.Components = append(out.Components, component)
		}
	}
	return out
}
func mergeBillingLabel(current, next string) string {
	head, tail, ok := strings.Cut(current, " · ")
	nextHead, nextTail, nextOK := strings.Cut(next, " · ")
	if !ok || !nextOK || tail != nextTail {
		return current
	}
	for _, name := range strings.Split(head, "、") {
		if name == nextHead {
			return current
		}
	}
	return head + "、" + nextHead + " · " + tail
}
func (m PublicModel) BillingComponents() []modelruntime.BillingComponent {
	index := map[string]int{}
	out := []modelruntime.BillingComponent{}
	for _, capability := range m.Capabilities {
		profile, ok := m.Billing[capability.Operation]
		if !ok {
			profile = modelruntime.DefaultBilling(capability.Operation)
		}
		for _, component := range profile.Components(capability.Operation) {
			if at, seen := index[component.Key]; seen {
				out[at].Label = mergeBillingLabel(out[at].Label, component.Label)
				continue
			}
			index[component.Key] = len(out)
			out = append(out, component)
		}
	}
	return out
}
func (s CommerceCatalog) UpdateBilling(ctx context.Context, actor, id string, op modelruntime.Operation, profile modelruntime.BillingProfile, expected uint64) error {
	if !s.platform(actor) {
		return ErrCatalogForbidden
	}
	if !profile.Valid(op) {
		return ErrInvalidInput
	}
	m, e := s.PublicModel(ctx, id)
	if e != nil {
		return e
	}
	found := false
	for _, c := range m.Capabilities {
		if c.Operation == op {
			found = true
		}
	}
	if !found {
		return ErrInvalidInput
	}
	if m.Billing == nil {
		m.Billing = map[modelruntime.Operation]modelruntime.BillingProfile{}
	}
	m.Billing[op] = profile
	return s.SavePublicModel(ctx, actor, m, expected)
}
