package catalog

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	StrategyPriority = "priority"
	StrategyWeight   = "weight"
	StrategyPrice    = "price"
	StrategyHealth   = "health"
)

func applyStrategy(cands []RouteCandidate, strategy string) {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case StrategyWeight:
		sort.SliceStable(cands, func(i, j int) bool {
			if cands[i].Weight != cands[j].Weight {
				return cands[i].Weight > cands[j].Weight
			}
			return cands[i].Priority < cands[j].Priority
		})
	case StrategyPrice:
		sort.SliceStable(cands, func(i, j int) bool {
			if cands[i].CostMinor != cands[j].CostMinor {
				return cands[i].CostMinor < cands[j].CostMinor
			}
			return cands[i].Priority < cands[j].Priority
		})
	case StrategyHealth:
		sort.SliceStable(cands, func(i, j int) bool {
			ri, rj := healthRank(cands[i].Health), healthRank(cands[j].Health)
			if ri != rj {
				return ri < rj
			}
			return cands[i].Priority < cands[j].Priority
		})
	default:
		sort.SliceStable(cands, func(i, j int) bool {
			if cands[i].Priority != cands[j].Priority {
				return cands[i].Priority < cands[j].Priority
			}
			return cands[i].Weight > cands[j].Weight
		})
	}
}

func healthRank(health string) int {
	switch strings.ToLower(health) {
	case "available":
		return 0
	case "degraded":
		return 1
	default:
		return 2
	}
}

func costMinor(raw []byte) int64 {
	if len(raw) == 0 {
		return math.MaxInt64 / 4
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return math.MaxInt64 / 4
	}
	for _, key := range []string{"upstream_cost_input", "input", "video_second_cost"} {
		if v, ok := body[key]; ok {
			if n := parsePriceToken(v); n > 0 {
				return n
			}
		}
	}
	return math.MaxInt64 / 4
}

func parsePriceToken(v any) int64 {
	switch n := v.(type) {
	case float64:
		if n <= 0 {
			return 0
		}
		return int64(math.Round(n * 1_000_000_000))
	case json.Number:
		f, err := n.Float64()
		if err != nil || f <= 0 {
			return 0
		}
		return int64(math.Round(f * 1_000_000_000))
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil || f <= 0 {
			return 0
		}
		return int64(math.Round(f * 1_000_000_000))
	default:
		return 0
	}
}
