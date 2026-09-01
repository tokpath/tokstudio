package catalog

import (
	"sort"
	"strconv"
	"strings"
)

// ModelListQuery 是公开目录的筛选条件。空字段表示不过滤。
type ModelListQuery struct {
	Vendor string
	Kind   string
	Q      string
	ID     string
	Limit  int
}

type FacetCount struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

type ModelFacets struct {
	Kinds   []FacetCount `json:"kinds"`
	Vendors []FacetCount `json:"vendors"`
}

type ModelListPage struct {
	Items  []ModelView
	Total  int
	Facets ModelFacets
}

var publicKindOrder = []string{"text", "image", "video", "embedding", "audio"}

// InferKind 与前端目录同一套规则：有 kind 用 kind，否则从 id/名称猜。
func InferKind(m ModelView) string {
	return inferKind(m.Kind, m.ID, m.DisplayName)
}

func inferKind(kind, id, displayName string) string {
	if k := strings.ToLower(strings.TrimSpace(kind)); k != "" && k != "all" {
		return k
	}
	hay := strings.ToLower(id + displayName)
	switch {
	case strings.Contains(hay, "seedance") || strings.Contains(hay, "wan-") || strings.Contains(hay, "happyhorse") || strings.Contains(hay, "video"):
		return "video"
	case strings.Contains(hay, "image") || strings.Contains(hay, "seedream") || strings.Contains(hay, "flux") || strings.Contains(hay, "banana") || strings.Contains(hay, "dall"):
		return "image"
	case strings.Contains(hay, "embed"):
		return "embedding"
	case strings.Contains(hay, "transcri") || strings.Contains(hay, "whisper") || strings.Contains(hay, "audio"):
		return "audio"
	default:
		return "text"
	}
}

func ParseLimit(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return 0
	}
	if n > 500 {
		return 500
	}
	return n
}

// ApplyPublicFilters 在渠道可见列表上做厂商/类型/搜索筛选，并给出分面计数。
// 类型分面跟着厂商+搜索走（不含当前类型）；厂商分面跟着类型+搜索走（不含当前厂商）。
func ApplyPublicFilters(items []ModelView, q ModelListQuery) ModelListPage {
	normalized := make([]ModelView, 0, len(items))
	for _, item := range items {
		item.Kind = InferKind(item)
		normalized = append(normalized, item)
	}

	vendor := strings.TrimSpace(q.Vendor)
	kind := strings.ToLower(strings.TrimSpace(q.Kind))
	if kind == "all" {
		kind = ""
	}
	needle := strings.ToLower(strings.TrimSpace(q.Q))
	id := strings.TrimSpace(q.ID)

	searched := make([]ModelView, 0, len(normalized))
	for _, item := range normalized {
		if id != "" && item.ID != id {
			continue
		}
		if needle != "" {
			hay := strings.ToLower(item.ID + " " + item.DisplayName + " " + item.Vendor + " " + item.Description)
			if !strings.Contains(hay, needle) {
				continue
			}
		}
		searched = append(searched, item)
	}

	kindFacets := countKinds(filterByVendor(searched, vendor))
	vendorFacets := countVendors(filterByKind(searched, kind))

	out := searched
	if vendor != "" {
		out = filterByVendor(out, vendor)
	}
	if kind != "" {
		out = filterByKind(out, kind)
	}

	total := len(out)
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	if out == nil {
		out = []ModelView{}
	}
	return ModelListPage{Items: out, Total: total, Facets: ModelFacets{Kinds: kindFacets, Vendors: vendorFacets}}
}

func filterByVendor(items []ModelView, vendor string) []ModelView {
	if vendor == "" {
		return items
	}
	out := make([]ModelView, 0, len(items))
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Vendor), vendor) {
			out = append(out, item)
		}
	}
	return out
}

func filterByKind(items []ModelView, kind string) []ModelView {
	if kind == "" {
		return items
	}
	out := make([]ModelView, 0, len(items))
	for _, item := range items {
		if InferKind(item) == kind {
			out = append(out, item)
		}
	}
	return out
}

func countKinds(items []ModelView) []FacetCount {
	counts := map[string]int{}
	for _, item := range items {
		counts[InferKind(item)]++
	}
	out := make([]FacetCount, 0, len(publicKindOrder))
	for _, id := range publicKindOrder {
		if n := counts[id]; n > 0 {
			out = append(out, FacetCount{ID: id, Count: n})
		}
	}
	for id, n := range counts {
		if n <= 0 || containsKind(publicKindOrder, id) {
			continue
		}
		out = append(out, FacetCount{ID: id, Count: n})
	}
	return out
}

func countVendors(items []ModelView) []FacetCount {
	counts := map[string]int{}
	for _, item := range items {
		vendor := strings.TrimSpace(item.Vendor)
		if vendor == "" {
			vendor = "unknown"
		}
		counts[vendor]++
	}
	out := make([]FacetCount, 0, len(counts))
	for id, n := range counts {
		out = append(out, FacetCount{ID: id, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return strings.ToLower(out[i].ID) < strings.ToLower(out[j].ID)
	})
	return out
}

func containsKind(list []string, id string) bool {
	for _, item := range list {
		if item == id {
			return true
		}
	}
	return false
}
