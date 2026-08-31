package catalog

import (
	"context"
	"crypto/sha1"
	"embed"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//go:embed seed/ofox-models.json seed/ofox-site.json
var ofoxSeedFS embed.FS

// OfoxModelSnapshot 是从 ofox 公开目录爬取后入库的价目行。
type OfoxModelSnapshot struct {
	ID                  string         `json:"id"`
	Vendor              string         `json:"vendor"`
	DisplayName         string         `json:"display_name"`
	Capabilities        map[string]any `json:"capabilities"`
	SellPrice           map[string]any `json:"sell_price"`
	Status              string         `json:"status"`
	Description         string         `json:"description"`
	ContextLength       int            `json:"context_length"`
	MaxCompletionTokens int            `json:"max_completion_tokens"`
	Kind                string         `json:"kind"`
	Created             int64          `json:"created"`
}

type ofoxSiteSnapshot struct {
	Source       string         `json:"source"`
	Fetched      string         `json:"fetched"`
	Leaderboards map[string]any `json:"leaderboards"`
	Apps         []any          `json:"apps"`
	Blog         []any          `json:"blog"`
	Discounts    map[string]any `json:"discounts"`
}

var cachedOfoxSite *ofoxSiteSnapshot

func ofoxStableID(prefix, publicID string) string {
	sum := sha1.Sum([]byte(publicID))
	return prefix + "_" + hex.EncodeToString(sum[:8])
}

func loadOfoxSnapshots() ([]OfoxModelSnapshot, error) {
	raw, err := ofoxSeedFS.ReadFile("seed/ofox-models.json")
	if err != nil {
		return nil, err
	}
	var items []OfoxModelSnapshot
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func loadOfoxSite() (*ofoxSiteSnapshot, error) {
	if cachedOfoxSite != nil {
		return cachedOfoxSite, nil
	}
	raw, err := ofoxSeedFS.ReadFile("seed/ofox-site.json")
	if err != nil {
		return nil, err
	}
	var site ofoxSiteSnapshot
	if err := json.Unmarshal(raw, &site); err != nil {
		return nil, err
	}
	cachedOfoxSite = &site
	return cachedOfoxSite, nil
}

// PublicSiteContent 返回爬取后的公开站内容（排行、博客、生态应用）。
func PublicSiteContent() map[string]any {
	site, err := loadOfoxSite()
	if err != nil {
		return map[string]any{"error": "site snapshot missing"}
	}
	return map[string]any{
		"source": site.Source, "fetched": site.Fetched,
		"leaderboards": site.Leaderboards, "apps": site.Apps,
		"blog": site.Blog, "discounts": site.Discounts,
	}
}

// ImportOfoxSnapshot 把 ofox 公开目录 dump 进 catalog 表，官方/渠道可见，OEM 白名单不变。
func (s *Service) ImportOfoxSnapshot(ctx context.Context) error {
	items, err := loadOfoxSnapshots()
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range items {
			if err := upsertOfoxModel(tx, items[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

func upsertOfoxModel(tx *gorm.DB, item OfoxModelSnapshot) error {
	publicID := strings.TrimSpace(item.ID)
	if publicID == "" || strings.HasPrefix(publicID, "tokenhub/") {
		return nil
	}
	caps := item.Capabilities
	if caps == nil {
		caps = map[string]any{}
	}
	if item.Description != "" {
		caps["description"] = item.Description
	}
	if item.Kind != "" {
		caps["kind"] = item.Kind
	}
	if item.ContextLength > 0 {
		caps["context_length"] = item.ContextLength
	}
	if item.MaxCompletionTokens > 0 {
		caps["max_completion_tokens"] = item.MaxCompletionTokens
	}
	if item.Created > 0 {
		caps["created"] = item.Created
	}
	capsJSON, _ := json.Marshal(caps)
	status := "published"
	if item.Status == "deprecated" {
		status = "deprecated"
	}
	model := publicModelRow{
		ID: ofoxStableID("mdl", publicID), PublicID: publicID,
		Vendor:       firstNonEmpty(item.Vendor, vendorFromID(publicID)),
		DisplayName:  firstNonEmpty(item.DisplayName, publicID),
		Capabilities: capsJSON, Status: status,
	}
	if err := tx.Where("public_id = ?", publicID).Assign(map[string]any{
		"vendor": model.Vendor, "display_name": model.DisplayName,
		"capabilities_json": model.Capabilities, "status": model.Status,
	}).FirstOrCreate(&model).Error; err != nil {
		return err
	}

	sell := map[string]any{}
	for k, v := range item.SellPrice {
		if isCostKey(k) {
			continue
		}
		sell[k] = stringifyPrice(v)
	}
	sell["currency"] = "USD"
	pricesJSON, _ := json.Marshal(sell)
	price := priceRow{
		ID: ofoxStableID("prc", publicID), PublicModelID: model.ID,
		UnitPrices: pricesJSON, Status: "published", EffectiveAt: time.Now().UTC(),
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"unit_prices_json", "status", "public_model_id"}),
	}).Create(&price).Error; err != nil {
		return err
	}

	for _, channelID := range []string{identity.OfficialChannelID, identity.ResellerChannelID} {
		if err := tx.Where("channel_org_id = ? AND public_model_id = ?", channelID, model.ID).
			FirstOrCreate(&channelPolicyRow{ChannelOrgID: channelID, PublicModelID: model.ID, Enabled: true}).Error; err != nil {
			return err
		}
	}
	return nil
}

func vendorFromID(publicID string) string {
	if i := strings.Index(publicID, "/"); i > 0 {
		return publicID[:i]
	}
	return "unknown"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func stringifyPrice(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	default:
		b, _ := json.Marshal(v)
		return strings.Trim(string(b), `"`)
	}
}

func isCostKey(k string) bool {
	k = strings.ToLower(k)
	return strings.Contains(k, "cost") || strings.Contains(k, "wholesale") || strings.Contains(k, "upstream")
}

func PublicModelStatus(status string) string {
	switch status {
	case "published":
		return "available"
	case "deprecated":
		return "unavailable"
	default:
		if status == "" {
			return "available"
		}
		return status
	}
}

func extraFromCaps(caps map[string]any) (desc, kind string, ctxLen, maxTok int) {
	if caps == nil {
		return
	}
	desc, _ = caps["description"].(string)
	kind, _ = caps["kind"].(string)
	ctxLen = asInt(caps["context_length"])
	maxTok = asInt(caps["max_completion_tokens"])
	return
}

func asInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(t)
		return n
	default:
		return 0
	}
}

func publicSell(price map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range price {
		if isCostKey(k) {
			continue
		}
		out[k] = v
	}
	return out
}
