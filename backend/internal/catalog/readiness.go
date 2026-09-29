package catalog

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// routeReadySQL is shared by public discovery and channel authorization.
// Health and account availability remain dynamic checks inside ResolveRoute.
const routeReadySQL = `EXISTS (
 SELECT 1 FROM catalog_route_groups rg
 JOIN catalog_route_candidates rc ON rc.route_group_id = rg.id
 JOIN catalog_provider_model_mappings mp ON mp.public_model_id = m.id AND mp.provider_id = rc.provider_id
 JOIN catalog_provider_models pm ON pm.provider_id = rc.provider_id AND pm.upstream_model_id = mp.upstream_model_id
 JOIN catalog_providers pr ON pr.id = rc.provider_id
 WHERE rg.public_model_id = m.id AND rg.status = 'active'
   AND mp.status = 'active' AND mp.upstream_model_id <> '' AND pr.status = 'active' AND pm.status = 'active'
   AND CASE COALESCE(m.capabilities_json->>'kind', 'text')
     WHEN 'embedding' THEN COALESCE(pm.unit_costs_json->>'input', '') <> ''
     WHEN 'image' THEN COALESCE(pm.unit_costs_json->>'image_count', '') <> ''
     WHEN 'video' THEN COALESCE(pm.unit_costs_json->>'video_second', '') <> ''
     WHEN 'audio' THEN COALESCE(pm.unit_costs_json->>'audio_second', '') <> ''
     ELSE COALESCE(pm.unit_costs_json->>'input', '') <> '' AND COALESCE(pm.unit_costs_json->>'output', '') <> ''
   END
)`

func (s *Service) validateModelReady(ctx context.Context, model publicModelRow) error {
	var caps map[string]any
	if err := json.Unmarshal(model.Capabilities, &caps); err != nil {
		return ErrModelIncomplete
	}
	var price priceRow
	if err := s.db.WithContext(ctx).Where("public_model_id = ? AND provider_id IS NULL AND status = ?", model.ID, SyncPublished).
		Order("effective_at DESC").First(&price).Error; err != nil {
		return ErrModelIncomplete
	}
	var units map[string]any
	if err := json.Unmarshal(price.UnitPrices, &units); err != nil {
		return ErrModelIncomplete
	}
	if !modelConfigurationReady(model, caps, units) {
		return ErrModelIncomplete
	}
	return nil
}

func modelConfigurationReady(model publicModelRow, caps, units map[string]any) bool {
	if strings.TrimSpace(model.PublicID) == "" || strings.TrimSpace(model.Vendor) == "" || strings.TrimSpace(model.DisplayName) == "" {
		return false
	}
	kind, _ := caps["kind"].(string)
	in, out := pickDim(units, "customer_sell", "customer_sell_input", "customer_sell_output", "input", "output")
	switch kind {
	case "text":
		return validPrice(in) && validPrice(out)
	case "embedding":
		return validPrice(in)
	case "image":
		return validPrice(stringifyPrice(units["image_count"]))
	case "video":
		return validPrice(stringifyPrice(units["video_second"]))
	case "audio":
		return validPrice(stringifyPrice(units["audio_second"]))
	}
	return false
}

func validPrice(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	n, err := strconv.ParseFloat(value, 64)
	return err == nil && n >= 0 && !math.IsNaN(n) && !math.IsInf(n, 0)
}

func (s *Service) routeReady(ctx context.Context, modelID string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Table("catalog_public_models m").Where("m.id = ?", modelID).Where(routeReadySQL).Count(&count).Error
	return count > 0, err
}
