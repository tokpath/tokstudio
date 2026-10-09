package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/identity"
)

// PriceForChannel fixes channel terms at reservation time. The public price
// version remains the catalog version; the effective terms are copied to usage.
func (s *Service) PriceForChannel(ctx context.Context, channelID, publicID string, raw json.RawMessage) (json.RawMessage, error) {
	prices := map[string]any{}
	if err := json.Unmarshal(raw, &prices); err != nil {
		return nil, err
	}
	for key := range prices {
		if strings.HasPrefix(key, "upstream_cost") || strings.HasPrefix(key, "wholesale") || strings.HasPrefix(key, "channel_customer") || strings.HasSuffix(key, "_cost") || key == "channel_override" || key == "channel_customer" {
			delete(prices, key)
		}
	}
	if channelID == "" {
		return json.Marshal(prices)
	}
	var policy channelPolicyRow
	err := s.db.WithContext(ctx).Table("catalog_channel_model_policies AS p").
		Select("p.*").Joins("JOIN catalog_public_models AS m ON m.id = p.public_model_id").
		Joins("JOIN identity_channel_orgs child ON child.id = p.channel_org_id").
		Joins("LEFT JOIN identity_channel_orgs parent ON parent.id = child.parent_id").
		Joins("LEFT JOIN catalog_channel_model_policies upstream ON upstream.channel_org_id = parent.id AND upstream.public_model_id = m.id").
		Where("p.channel_org_id = ? AND m.public_id = ? AND m.status = 'published' AND p.enabled = true AND p.self_enabled = true", channelID, publicID).
		Where("(parent.type IS DISTINCT FROM ? OR (upstream.enabled = true AND upstream.self_enabled = true))", identity.ChannelTypeC).First(&policy).Error
	if err != nil {
		return nil, err
	}
	for key, value := range decodeCosts(policy.Wholesale) {
		prices["wholesale_"+key] = value
	}
	customerOverride := decodeCosts(policy.Override)
	var channel struct {
		Type string `gorm:"column:type"`
	}
	if err := s.db.WithContext(ctx).Table("identity_channel_orgs").Select("type").Where("id = ?", channelID).Take(&channel).Error; err != nil {
		return nil, err
	}
	if channel.Type == identity.ChannelTypeB {
		customerOverride = nil
		parentID, err := s.delegatingParent(ctx, channelID)
		if err != nil {
			return nil, err
		}
		if parentID != "" {
			var parentPolicy channelPolicyRow
			if err := s.db.WithContext(ctx).Where("channel_org_id = ? AND public_model_id = ?", parentID, policy.PublicModelID).First(&parentPolicy).Error; err != nil {
				return nil, err
			}
			customerOverride = decodeCosts(parentPolicy.Override)
		}
	}
	for key, value := range customerOverride {
		if key == "input" || key == "output" {
			prices[key] = value
			prices["customer_sell_"+key] = value
		} else {
			prices[key] = value
		}
	}
	if _, ok := customerOverride["input"]; ok {
		sell := map[string]any{}
		for _, key := range []string{"input", "output"} {
			if value, exists := prices[key]; exists {
				sell[key] = value
			}
		}
		prices["customer_sell"] = sell
	}
	return json.Marshal(prices)
}

// SetOwnChannelCustomerPrices sets the selling terms for one OEM brand.
// Child B policies never define a separate customer price.
func (s *Service) SetOwnChannelCustomerPrices(ctx context.Context, channelID, publicID string, prices map[string]string) error {
	var organization struct{ Type string }
	if err := s.db.WithContext(ctx).Table("identity_channel_orgs").Select("type").Where("id=?", channelID).Take(&organization).Error; err != nil {
		return err
	}
	if organization.Type != identity.ChannelTypeC {
		return ErrModelNotVisible
	}
	var model publicModelRow
	if err := s.db.WithContext(ctx).Where("public_id = ?", publicID).First(&model).Error; err != nil {
		return err
	}
	validated, err := validateUnitCosts(prices)
	if err != nil || (len(validated) > 0 && !pricedForKind(validated, modelKind(model))) {
		return ErrInvalidInput
	}
	encoded, _ := json.Marshal(validated)
	result := s.db.WithContext(ctx).Model(&channelPolicyRow{}).Where("channel_org_id = ? AND public_model_id = ? AND enabled = true", channelID, model.ID).Update("customer_override_json", encoded)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrModelNotVisible
	}
	return nil
}

// PriceWithProviderCosts replaces the old model-wide estimate with the costs
// of the provider/model that actually completed the request.
func PriceWithProviderCosts(raw, costs json.RawMessage) (json.RawMessage, error) {
	if len(costs) == 0 {
		return nil, ErrProviderModelUnpriced
	}
	prices := map[string]any{}
	if err := json.Unmarshal(raw, &prices); err != nil {
		return nil, err
	}
	for _, key := range []string{"upstream_cost_input", "upstream_cost_output", "image_count_cost", "video_second_cost", "audio_second_cost"} {
		delete(prices, key)
	}
	delete(prices, "upstream_cost")
	var source map[string]string
	if err := json.Unmarshal(costs, &source); err != nil {
		return nil, err
	}
	for key, value := range source {
		switch key {
		case "input", "output":
			prices["upstream_cost_"+key] = value
		case "image_count", "video_second", "audio_second":
			prices[key+"_cost"] = value
		}
	}
	return json.Marshal(prices)
}

// PriceSnapshot 是账务模块允许看到的公开价格视图，不含内部 ORM。
type PriceSnapshot struct {
	VersionID   string          `json:"version_id"`
	PublicID    string          `json:"public_id"`
	Raw         json.RawMessage `json:"unit_prices"`
	EffectiveAt time.Time       `json:"effective_at"`
}

type PriceBookView struct {
	ID          string          `json:"id"`
	PublicID    string          `json:"public_id"`
	Status      string          `json:"status"`
	UnitPrices  json.RawMessage `json:"unit_prices"`
	EffectiveAt time.Time       `json:"effective_at"`
	Sell        string          `json:"sell"`
}

var ErrEmptyUnitPrices = errors.New("unit prices required")

// NormalizeUnitPrices only accepts the public selling price. Provider costs
// and channel terms are configured on their own resources.
func NormalizeUnitPrices(in map[string]any) (map[string]any, error) {
	if in == nil {
		return nil, ErrEmptyUnitPrices
	}
	out := map[string]any{}

	sellIn, sellOut := pickDim(in, "customer_sell", "customer_sell_input", "customer_sell_output", "input", "output")

	writeDim(out, "customer_sell", "customer_sell_input", "customer_sell_output", sellIn, sellOut)
	if sellIn != "" {
		out["input"] = sellIn
	}
	if sellOut != "" {
		out["output"] = sellOut
	}

	if curr := stringifyPrice(in["currency"]); curr != "" {
		if curr != "USD" {
			return nil, ErrInvalidInput
		}
		out["currency"] = curr
	} else {
		out["currency"] = "USD"
	}
	for _, key := range []string{
		"video_second", "image_count", "audio_second",
		"reasoning", "reasoning_output",
	} {
		if v := stringifyPrice(in[key]); v != "" {
			out[key] = v
		}
	}

	if !hasPricedUnit(out) {
		return nil, ErrEmptyUnitPrices
	}
	for _, key := range []string{"input", "output", "video_second", "image_count", "audio_second", "reasoning", "reasoning_output"} {
		if value := stringifyPrice(out[key]); value != "" && !validPrice(value) {
			return nil, ErrInvalidInput
		}
	}
	return out, nil
}

func mergeUnitPrices(prev, incoming map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range prev {
		switch k {
		case "input", "output", "customer_sell", "customer_sell_input", "customer_sell_output", "currency", "video_second", "image_count", "audio_second", "reasoning", "reasoning_output":
			out[k] = v
		}
	}
	for k, v := range incoming {
		out[k] = v
	}
	return out
}

func priceBookFromRow(row priceRow, publicID string) PriceBookView {
	dims := decodeUnitPrices(row.UnitPrices)
	return PriceBookView{
		ID:          row.ID,
		PublicID:    publicID,
		Status:      row.Status,
		UnitPrices:  json.RawMessage(row.UnitPrices),
		EffectiveAt: row.EffectiveAt,
		Sell:        formatIO(firstNonEmpty(dims["input"], dims["customer_sell_input"]), firstNonEmpty(dims["output"], dims["customer_sell_output"])),
	}
}

func decodeUnitPrices(raw []byte) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return out
	}
	sellIn, sellOut := pickDim(m, "customer_sell", "customer_sell_input", "customer_sell_output", "input", "output")
	costIn, costOut := pickDim(m, "upstream_cost", "upstream_cost_input", "upstream_cost_output")
	wholeIn, wholeOut := pickDim(m, "wholesale", "wholesale_input", "wholesale_output")
	chIn, chOut := pickDim(m, "channel_override", "channel_customer_input", "channel_customer_output")
	if chIn == "" && chOut == "" {
		chIn, chOut = pickDim(m, "channel_customer", "channel_customer_price_input", "channel_customer_price_output")
	}
	set := func(k, v string) {
		if v != "" {
			out[k] = v
		}
	}
	set("input", sellIn)
	set("output", sellOut)
	set("customer_sell_input", sellIn)
	set("customer_sell_output", sellOut)
	set("upstream_cost_input", costIn)
	set("upstream_cost_output", costOut)
	set("wholesale_input", wholeIn)
	set("wholesale_output", wholeOut)
	set("channel_customer_input", chIn)
	set("channel_customer_output", chOut)
	return out
}

func pickDim(in map[string]any, nested string, keys ...string) (input, output string) {
	if nested != "" {
		if obj, ok := asObject(in[nested]); ok {
			input = stringifyPrice(firstKey(obj, "input", "in"))
			output = stringifyPrice(firstKey(obj, "output", "out"))
		}
	}
	for _, key := range keys {
		if strings.HasSuffix(key, "_output") || key == "output" {
			if output == "" {
				output = stringifyPrice(in[key])
			}
			continue
		}
		if input == "" {
			input = stringifyPrice(in[key])
		}
	}
	return input, output
}

func writeDim(out map[string]any, nested, inKey, outKey, input, output string) {
	if input == "" && output == "" {
		return
	}
	dim := map[string]any{}
	if input != "" {
		out[inKey] = input
		dim["input"] = input
	}
	if output != "" {
		out[outKey] = output
		dim["output"] = output
	}
	out[nested] = dim
}

func asObject(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func firstKey(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return nil
}

func hasPricedUnit(out map[string]any) bool {
	for _, key := range []string{
		"input", "output", "customer_sell_input", "customer_sell_output",
		"video_second", "image_count", "audio_second",
	} {
		if stringifyPrice(out[key]) != "" {
			return true
		}
	}
	return false
}

// FourPriceDims 从快照 JSON 抽出四列单价（in/out）。渠道覆盖可以缺省。
func FourPriceDims(raw []byte) (upstream, wholesale, sell, channel string) {
	d := decodeUnitPrices(raw)
	return formatIO(d["upstream_cost_input"], d["upstream_cost_output"]),
		formatIO(d["wholesale_input"], d["wholesale_output"]),
		formatIO(firstNonEmpty(d["input"], d["customer_sell_input"]), firstNonEmpty(d["output"], d["customer_sell_output"])),
		formatIO(d["channel_customer_input"], d["channel_customer_output"])
}

func RequireFourPriceSnapshot(raw []byte, channelRequired bool) error {
	upstream, wholesale, sell, channel := FourPriceDims(raw)
	if upstream == "" || wholesale == "" || sell == "" {
		return errors.New("usage snapshot missing upstream/wholesale/sell")
	}
	if channelRequired && channel == "" {
		return errors.New("usage snapshot missing optional channel that was published")
	}
	return nil
}

func formatIO(input, output string) string {
	switch {
	case input == "" && output == "":
		return ""
	case output == "":
		return input
	case input == "":
		return output
	default:
		return input + "/" + output
	}
}

// ValidateChannelPrice checks the effective brand sell and OEM settlement terms
// without inspecting or changing historical snapshots.
func (s *Service) ValidateChannelPrice(ctx context.Context, channelID, publicID string, raw json.RawMessage) error {
	model, err := s.loadModel(ctx, publicID)
	if err != nil {
		return err
	}
	var units map[string]any
	if json.Unmarshal(raw, &units) != nil {
		return ErrInvalidInput
	}
	if !modelConfigurationReady(*model, map[string]any{"kind": modelKind(*model)}, units) {
		return ErrModelIncomplete
	}
	wholesale := map[string]string{}
	for _, key := range costKeys {
		wholesale[key] = stringifyPrice(units["wholesale_"+key])
	}
	if !pricedForKind(wholesale, modelKind(*model)) {
		return ErrModelIncomplete
	}
	return nil
}
