package catalog

import "strings"

type mappedModelScan struct {
	ProviderID      string `gorm:"column:provider_id"`
	PublicID        string `gorm:"column:public_id"`
	Vendor          string `gorm:"column:vendor"`
	DisplayName     string `gorm:"column:display_name"`
	UpstreamModelID string `gorm:"column:upstream_model_id"`
	Status          string `gorm:"column:status"`
}

func FilterProviders(items []ProviderView, q string) []ProviderView {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return items
	}
	out := make([]ProviderView, 0, len(items))
	for _, item := range items {
		hay := strings.ToLower(item.ID + " " + item.Name + " " + item.Slug + " " + item.Kind + " " + item.Adapter + " " + item.Status + " " + item.Health)
		for _, model := range item.Models {
			hay += " " + strings.ToLower(model.PublicID+" "+model.Vendor+" "+model.DisplayName+" "+model.UpstreamModelID)
		}
		if strings.Contains(hay, q) {
			out = append(out, item)
		}
	}
	return out
}

func attachMappedModels(items []ProviderView, rows []mappedModelScan) []ProviderView {
	byProvider := make(map[string][]MappedModelView, len(items))
	for _, row := range rows {
		byProvider[row.ProviderID] = append(byProvider[row.ProviderID], MappedModelView{
			PublicID:        row.PublicID,
			Vendor:          row.Vendor,
			DisplayName:     row.DisplayName,
			UpstreamModelID: row.UpstreamModelID,
			Status:          row.Status,
		})
	}
	for i := range items {
		models := byProvider[items[i].ID]
		if models == nil {
			models = []MappedModelView{}
		}
		items[i].Models = models
	}
	return items
}
