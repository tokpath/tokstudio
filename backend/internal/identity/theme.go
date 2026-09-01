package identity

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

const (
	DefaultBrand           = "#2150D6"
	DefaultBrandPress      = "#183CA8"
	DefaultBrandSoft       = "#DCE6FB"
	DefaultBrandSoftDark   = "#243056"
	DefaultBrandEmphasis   = "#2150D6"
	DefaultBrandEmphasisDk = "#8AA4FF"
	DefaultOnBrand         = "#FFFFFF"
	PaperCanvas            = "#F4F1EA"
	CarbonCanvas           = "#161513"
	minContrast            = 4.5
)

var hexColor = regexp.MustCompile(`(?i)^#[0-9a-f]{6}$`)

var themeAllowed = map[string]struct{}{
	"display_name": {}, "logo_url": {}, "favicon_url": {}, "default_theme": {},
	"brand": {}, "brand_press": {}, "brand_soft": {}, "brand_soft_dark": {},
	"brand_emphasis": {}, "brand_emphasis_dark": {}, "on_brand": {},
}

func NormalizeTheme(raw map[string]any) (map[string]string, error) {
	out := map[string]string{
		"brand": DefaultBrand, "brand_press": DefaultBrandPress,
		"brand_soft": DefaultBrandSoft, "brand_soft_dark": DefaultBrandSoftDark,
		"brand_emphasis": DefaultBrandEmphasis, "brand_emphasis_dark": DefaultBrandEmphasisDk,
		"on_brand": DefaultOnBrand, "default_theme": "system",
	}
	if raw == nil {
		return out, nil
	}
	for key, value := range raw {
		if _, ok := themeAllowed[key]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrThemeKey, key)
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			continue
		}
		switch key {
		case "default_theme":
			if text != "light" && text != "dark" && text != "system" {
				return nil, fmt.Errorf("%w: default_theme", ErrThemeKey)
			}
			out[key] = text
		case "display_name", "logo_url", "favicon_url":
			out[key] = text
		default:
			if !hexColor.MatchString(text) {
				return nil, fmt.Errorf("%w: %s", ErrThemeHex, key)
			}
			out[key] = strings.ToUpper(text)
		}
	}
	if err := assertContrast(out["on_brand"], out["brand"]); err != nil {
		return nil, err
	}
	if err := assertContrast(out["brand_emphasis"], PaperCanvas); err != nil {
		return nil, err
	}
	if err := assertContrast(out["brand_emphasis_dark"], CarbonCanvas); err != nil {
		return nil, err
	}
	return out, nil
}

func themeBytes(theme map[string]string) []byte {
	raw, _ := json.Marshal(theme)
	return raw
}

func assertContrast(fg, bg string) error {
	ratio := contrastRatio(fg, bg)
	if ratio+1e-9 < minContrast {
		return fmt.Errorf("%w: %s on %s = %.2f", ErrThemeContrast, fg, bg, ratio)
	}
	return nil
}

func contrastRatio(a, b string) float64 {
	l1, l2 := relativeLuminance(a), relativeLuminance(b)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

func relativeLuminance(hex string) float64 {
	r, g, b := parseHex(hex)
	return 0.2126*linearize(r) + 0.7152*linearize(g) + 0.0722*linearize(b)
}

func parseHex(hex string) (float64, float64, float64) {
	hex = strings.TrimPrefix(strings.ToUpper(hex), "#")
	if len(hex) != 6 {
		return 0, 0, 0
	}
	var n int
	fmt.Sscanf(hex, "%06X", &n)
	return float64((n>>16)&0xFF) / 255, float64((n>>8)&0xFF) / 255, float64(n&0xFF) / 255
}

func linearize(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}
