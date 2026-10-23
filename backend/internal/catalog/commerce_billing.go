package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
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
	var pending []CatalogChange
	if e = s.DB.WithContext(ctx).Where("operator_id=? AND kind='model' AND resource_id=? AND expected_version=? AND state IN ('announcing','announced')", s.PlatformOperatorID, model.ID, model.Version).Order("created_at DESC,id DESC").Limit(1).Find(&pending).Error; e != nil {
		return e
	}
	if len(pending) > 0 {
		return s.preparePendingBillingPrice(ctx, actor, model, &pending[0], v)
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

// Prices can be prepared while a template is being announced. Sizes belong to
// that pending template, so saving a price must not advance the live model CAS
// and invalidate its pending change. Old meters retain their previous rates
// until the runtime switches templates; only the selected profile is billed.
func (s CommerceCatalog) preparePendingBillingPrice(ctx context.Context, actor string, current PublicModel, change *CatalogChange, price *PriceVersion) error {
	var next PublicModel
	if e := json.Unmarshal(change.Payload, &next); e != nil {
		return e
	}
	if next.Billing == nil {
		next.Billing = current.Billing
	}
	changed, e := applyResolutionPrices(&next, price.Rule.Components)
	if e != nil {
		return e
	}
	if changed {
		if !s.platform(actor) {
			return ErrResolutionPrice
		}
		raw, e := json.Marshal(next)
		if e != nil {
			return e
		}
		if e = s.DB.WithContext(ctx).Model(change).Updates(map[string]any{"payload": raw, "fingerprint": changeFingerprint("model", next.ID, raw)}).Error; e != nil {
			return e
		}
	}
	previous, e := (PriceResolver{Source: billingRegularSource{CommerceCatalog: s, Scope: price.Scope}}).Resolve(ctx, price.Scope, current.ID, price.EffectiveAt)
	if errors.Is(e, ErrPriceUnavailable) {
		return nil
	}
	if e != nil {
		return e
	}
	factor, e := previous.Factor()
	if e != nil {
		return e
	}
	wanted, provided := map[string]bool{}, map[string]bool{}
	for _, component := range current.BillingComponents() {
		wanted[component.Key] = true
	}
	for _, component := range price.Rule.Components {
		provided[component.Key] = true
	}
	for _, component := range previous.Components {
		if !wanted[component.Key] || provided[component.Key] {
			continue
		}
		rate, e := PriceDecimal(component.Rate)
		if e != nil {
			return e
		}
		component.Rate, e = exactPublicDecimal(new(big.Rat).Mul(rate, factor))
		if e != nil {
			return e
		}
		price.Rule.Components = append(price.Rule.Components, component)
		provided[component.Key] = true
	}
	return nil
}

type billingRegularSource struct {
	CommerceCatalog
	Scope PriceScope
}

func (s billingRegularSource) PriceVersions(ctx context.Context, scope PriceScope, model string) ([]PriceVersion, error) {
	rows, e := s.CommerceCatalog.PriceVersions(ctx, scope, model)
	if e != nil || scope != s.Scope {
		return rows, e
	}
	regular := []PriceVersion{}
	for _, row := range rows {
		if row.Layer == "regular" {
			regular = append(regular, row)
		}
	}
	return regular, nil
}
