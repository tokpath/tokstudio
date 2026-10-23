package modelruntime

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Public measuring contracts are independent of supplier cost. A direction
// either uses aggregate tokens or disjoint modality items, never both.
type BillingVariant struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Size    string `json:"size,omitempty"`
	Quality string `json:"quality,omitempty"`
}
type BillingItem struct {
	Modality string           `json:"modality"`
	Template string           `json:"template"`
	Variants []BillingVariant `json:"variants,omitempty"`
}
type BillingProfile struct {
	Input  []BillingItem `json:"input"`
	Output []BillingItem `json:"output"`
}
type BillingComponent struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Unit  string `json:"unit"`
	Per   string `json:"per"`
}

func DefaultBilling(op Operation) BillingProfile {
	switch op {
	case Image:
		return BillingProfile{Output: []BillingItem{{Modality: "image", Template: "images"}}}
	case Speech:
		return BillingProfile{Input: []BillingItem{{Modality: "text", Template: "characters"}}}
	case Transcription:
		return BillingProfile{Input: []BillingItem{{Modality: "audio", Template: "seconds"}}}
	case Video:
		return BillingProfile{Output: []BillingItem{{Modality: "video", Template: "seconds"}}}
	case Embedding:
		return BillingProfile{Input: []BillingItem{{Modality: "all", Template: "tokens"}}}
	default:
		return BillingProfile{Input: []BillingItem{{Modality: "all", Template: "tokens"}}, Output: []BillingItem{{Modality: "all", Template: "tokens"}}}
	}
}
func (p BillingProfile) Valid(op Operation) bool {
	switch op {
	case Chat, Responses, Embedding, Image, Speech, Transcription, Video:
	default:
		return false
	}
	if len(p.Input)+len(p.Output) == 0 || len(p.Input) > 4 || len(p.Output) > 4 || op == Embedding && len(p.Output) > 0 {
		return false
	}
	for _, direction := range []string{"input", "output"} {
		items := p.Items(direction)
		seen := map[string]bool{}
		for _, item := range items {
			if seen[item.Modality] || item.Modality == "all" && len(items) > 1 {
				return false
			}
			seen[item.Modality] = true
			allowed := false
			switch item.Modality {
			case "all":
				allowed = item.Template == "tokens"
			case "text":
				allowed = item.Template == "tokens" || item.Template == "characters"
			case "image":
				allowed = item.Template == "tokens" || item.Template == "images" || item.Template == "megapixels"
			case "audio", "video":
				allowed = item.Template == "tokens" || item.Template == "seconds"
			}
			if !allowed || len(item.Variants) > 32 || len(item.Variants) > 0 && (item.Modality != "image" && item.Modality != "video" || item.Template == "tokens") {
				return false
			}
			ids := map[string]bool{}
			for index, v := range item.Variants {
				if v.ID == "" || len(v.ID) > 80 || strings.ContainsAny(v.ID, " :/@") || ids[v.ID] || strings.TrimSpace(v.Name) == "" || v.Size == "" && v.Quality == "" || direction == "input" && v.Quality != "" {
					return false
				}
				ids[v.ID] = true
				if v.Size != "" {
					var w, h int64
					if !parseSize(v.Size, &w, &h) {
						return false
					}
				}
				for _, previous := range item.Variants[:index] {
					if (previous.Size == "" || v.Size == "" || previous.Size == v.Size) && (previous.Quality == "" || v.Quality == "" || previous.Quality == v.Quality) {
						return false
					}
				}
			}
		}
	}
	return true
}
func (p BillingProfile) Items(direction string) []BillingItem {
	if direction == "input" {
		return p.Input
	}
	return p.Output
}
func (p BillingProfile) Has(direction, modality, template string) bool {
	for _, item := range p.Items(direction) {
		if item.Modality == modality && item.Template == template {
			return true
		}
	}
	return false
}
func (item BillingItem) Key(direction string) string {
	if item.Modality == "all" && item.Template == "tokens" {
		return direction
	}
	suffix := map[string]string{"images": "images", "megapixels": "megapixels", "characters": "characters", "seconds": "seconds", "tokens": "tokens"}[item.Template]
	return direction + "_" + item.Modality + "_" + suffix
}
func capabilityName(op Operation) string {
	switch op {
	case Chat:
		return "对话"
	case Responses:
		return "Responses"
	case Embedding:
		return "向量"
	case Image:
		return "图片生成"
	case Speech:
		return "语音生成"
	case Transcription:
		return "语音转写"
	case Video:
		return "视频生成"
	default:
		return string(op)
	}
}
func (item BillingItem) meterLabel(direction string) string {
	if item.Modality == "all" {
		if direction == "input" {
			return "输入（全部按 Token）"
		}
		return "输出（全部按 Token）"
	}
	modality := map[string]string{"text": "文本", "image": "图片", "audio": "音频", "video": "视频"}[item.Modality]
	if direction == "output" {
		return "输出" + modality
	}
	return "输入" + modality
}
func (item BillingItem) Components(direction string) []BillingComponent {
	return item.components(direction, "")
}
func (item BillingItem) components(direction string, op Operation) []BillingComponent {
	unit, per := map[string]string{"tokens": "token", "images": "image", "megapixels": "megapixel", "characters": "character", "seconds": "second"}[item.Template], "1"
	if item.Template == "tokens" || item.Template == "characters" {
		per = "1000000"
	}
	key := item.Key(direction)
	label := item.meterLabel(direction)
	cacheLabel := "缓存读取"
	if op != "" {
		label = capabilityName(op) + " · " + label
		cacheLabel = capabilityName(op) + " · 缓存读取"
	}
	if len(item.Variants) > 0 {
		// Listed resolutions are priced on their own. The unsuffixed key remains
		// the price for every other measured size.
		out := []BillingComponent{}
		for _, v := range item.Variants {
			out = append(out, BillingComponent{key + ":" + v.ID, label + " · " + v.Name, unit, per})
		}
		out = append(out, BillingComponent{key, label, unit, per})
		return out
	}
	out := []BillingComponent{{key, label, unit, per}}
	if direction == "input" && item.Template == "tokens" {
		cache := "cache_" + item.Modality
		if item.Modality == "all" {
			cache = "cache"
		}
		out = append(out, BillingComponent{cache, cacheLabel, "token", "1000000"})
	}
	return out
}
func (p BillingProfile) Components(op Operation) []BillingComponent {
	out := []BillingComponent{}
	for _, direction := range []string{"input", "output"} {
		for _, item := range p.Items(direction) {
			out = append(out, item.components(direction, op)...)
		}
	}
	return out
}
func (p BillingProfile) Capability(op Operation, stream bool) Capability {
	c := Capability{Operation: op, Streaming: stream, Billing: &p, BillableBounds: map[string]string{}}
	for _, m := range p.Components(op) {
		bound := "1"
		switch m.Unit {
		case "token":
			bound = "4096"
			if strings.HasPrefix(m.Key, "input") || strings.HasPrefix(m.Key, "cache") {
				bound = "131072"
			}
		case "second":
			bound = "60"
		case "character":
			bound = "10000"
		case "megapixel":
			bound = "4"
		}
		c.UsageRules = append(c.UsageRules, UsageRule{Component: m.Key, Unit: m.Unit, Path: "/" + m.Key})
		c.BillableBounds[m.Key] = bound
	}
	return c
}
func CanonicalSize(size string) (string, bool) {
	var width, height int64
	if !parseSize(size, &width, &height) {
		return "", false
	}
	return strconv.FormatInt(width, 10) + "x" + strconv.FormatInt(height, 10), true
}

// SplitResolution reports the meter key and canonical size encoded as key:WxH.
func SplitResolution(key string) (string, string, bool) {
	parent, size, ok := strings.Cut(key, ":")
	if !ok || parent == "" || strings.Contains(size, ":") {
		return "", "", false
	}
	canonical, ok := CanonicalSize(size)
	if !ok || canonical != size {
		return "", "", false
	}
	return parent, canonical, true
}

// EnsureSize adds a resolution that can be priced on its own. An existing
// variant with the same size keeps its identifier so older prices still match.
func (item *BillingItem) EnsureSize(size string) (string, bool, error) {
	canonical, ok := CanonicalSize(size)
	if !ok || canonical != size || strings.ContainsAny(size, " :/@") || len(size) > 80 {
		return "", false, ErrInvalidRequest
	}
	found := ""
	for _, variant := range item.Variants {
		if variant.ID != size && variant.Size != size {
			continue
		}
		if found != "" && found != variant.ID {
			return "", false, fmt.Errorf("%w: ambiguous billing specification", ErrUnsupported)
		}
		found = variant.ID
	}
	if found != "" {
		return found, false, nil
	}
	if len(item.Variants) >= 32 {
		return "", false, ErrInvalidRequest
	}
	item.Variants = append(item.Variants, BillingVariant{ID: size, Name: size, Size: size})
	return size, true, nil
}

func (item BillingItem) SelectVariant(params json.RawMessage) (string, error) {
	if len(item.Variants) == 0 {
		return "", nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(params, &fields) != nil {
		return "", ErrInvalidRequest
	}
	value := func(key string) string {
		var text string
		_ = json.Unmarshal(fields[key], &text)
		return text
	}
	size, quality := value("size"), value("quality")
	best, selected := -1, ""
	for _, variant := range item.Variants {
		if variant.Size != "" && variant.Size != size || variant.Quality != "" && variant.Quality != quality {
			continue
		}
		score := 0
		if variant.Size != "" {
			score += 2
		}
		if variant.Quality != "" {
			score++
		}
		if score < best {
			continue
		}
		if score == best {
			return "", fmt.Errorf("%w: ambiguous billing specification", ErrUnsupported)
		}
		best, selected = score, variant.ID
	}
	// No listed resolution matches. Callers bill the unsuffixed meter instead.
	return selected, nil
}
func (p BillingProfile) ValidateSpecifications(params json.RawMessage) error {
	for _, item := range p.Output {
		if _, e := item.SelectVariant(params); e != nil {
			return e
		}
	}
	return nil
}
func BillingParameters(params json.RawMessage) json.RawMessage {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(params, &fields)
	out := map[string]json.RawMessage{}
	for _, key := range []string{"size", "quality"} {
		if value, ok := fields[key]; ok {
			out[key] = value
		}
	}
	raw, _ := json.Marshal(out)
	return raw
}
