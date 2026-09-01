package identity

import "testing"

func TestNormalizeThemeRejectsForbiddenKeys(t *testing.T) {
	_, err := NormalizeTheme(map[string]any{"paper": "#F4F1EA"})
	if err == nil {
		t.Fatal("expected paper key to fail")
	}
}

func TestNormalizeThemeRejectsLowContrast(t *testing.T) {
	_, err := NormalizeTheme(map[string]any{"brand": "#F5E6A8", "on_brand": "#FFFFFF"})
	if err == nil {
		t.Fatal("pale yellow with white text must fail contrast")
	}
}

func TestNormalizeThemeOEMAmber(t *testing.T) {
	theme, err := NormalizeTheme(map[string]any{
		"brand": "#92400E", "brand_press": "#7C2D12", "brand_emphasis": "#92400E",
		"brand_emphasis_dark": "#F0B27A", "on_brand": "#FFFFFF",
	})
	if err != nil {
		t.Fatal(err)
	}
	if theme["brand"] != "#92400E" {
		t.Fatalf("brand %s", theme["brand"])
	}
}
