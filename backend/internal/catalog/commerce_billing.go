package catalog

import (
	"context"
	"strings"

	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

func activePrice(price ResolvedPrice, keys map[string]string) ResolvedPrice {
	needed := map[string]bool{}
	for key := range keys {
		needed[key] = true
		if parent, ok := resolutionParent(key); ok {
			needed[parent] = true
		}
	}
	out := price
	out.Components = nil
	for _, component := range price.Components {
		if needed[component.Key] {
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

// applyResolutionPrices records each WxH price on the matching image or video
// meter. A size that already exists keeps that variant's identifier.
func applyResolutionPrices(model *PublicModel, components []ComponentRate) (bool, error) {
	if model.Billing == nil {
		model.Billing = map[modelruntime.Operation]modelruntime.BillingProfile{}
	}
	changed := false
	for i := range components {
		parent, size, ok := modelruntime.SplitResolution(components[i].Key)
		if !ok {
			continue
		}
		id, added, e := ensureModelSize(model, parent, size)
		if e != nil {
			return false, e
		}
		if id != size {
			components[i].Key = parent + ":" + id
		}
		changed = changed || added
	}
	return changed, nil
}
func ensureModelSize(model *PublicModel, parent, size string) (string, bool, error) {
	for _, capability := range model.Capabilities {
		profile, ok := model.Billing[capability.Operation]
		if !ok {
			profile = modelruntime.DefaultBilling(capability.Operation)
		}
		for _, direction := range []string{"input", "output"} {
			items := profile.Input
			if direction == "output" {
				items = profile.Output
			}
			for index := range items {
				if items[index].Key(direction) != parent {
					continue
				}
				if items[index].Template == "tokens" || (items[index].Modality != "image" && items[index].Modality != "video") {
					return "", false, ErrInvalidInput
				}
				id, added, e := items[index].EnsureSize(size)
				if e != nil {
					return "", false, ErrInvalidInput
				}
				if direction == "output" {
					profile.Output = items
				} else {
					profile.Input = items
				}
				if !profile.Valid(capability.Operation) {
					return "", false, ErrInvalidInput
				}
				model.Billing[capability.Operation] = profile
				return id, added, nil
			}
		}
	}
	return "", false, ErrInvalidInput
}
func (s CommerceCatalog) syncResolutionPrices(ctx context.Context, actor string, v *PriceVersion) error {
	model, e := s.PublicModel(ctx, v.ModelID)
	if e != nil {
		return e
	}
	changed, e := applyResolutionPrices(&model, v.Rule.Components)
	if e != nil || !changed {
		return e
	}
	if !s.platform(actor) {
		return ErrResolutionPrice
	}
	// The price publication is the notice. Resolution meters have to exist at
	// the same moment, including for a model that is already open.
	return s.savePublicModel(context.WithValue(ctx, catalogApplyKey{}, true), actor, model, model.Version)
}
