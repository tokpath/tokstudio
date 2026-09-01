package identity

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"testing"
)

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestValidateAssetLogoContract(t *testing.T) {
	ok, err := ValidateAsset(AssetLogo, "logo.png", pngOf(t, 256, 256))
	if err != nil {
		t.Fatal(err)
	}
	if ok.Width != 256 || ok.ContentType != "image/png" {
		t.Fatalf("%+v", ok)
	}
	if _, err := ValidateAsset(AssetLogo, "tiny.png", pngOf(t, 32, 32)); err != ErrAssetDimension {
		t.Fatalf("32px logo: %v", err)
	}
	if _, err := ValidateAsset(AssetLogo, "wide.png", pngOf(t, 800, 100)); err != ErrAssetDimension {
		t.Fatalf("8:1 logo: %v", err)
	}
	if _, err := ValidateAsset("hero", "logo.png", pngOf(t, 256, 256)); err != ErrAssetKind {
		t.Fatalf("kind: %v", err)
	}
	if _, err := ValidateAsset(AssetLogo, "big.bin", bytes.Repeat([]byte("x"), LogoMaxBytes+1)); err != ErrAssetTooLarge {
		t.Fatalf("size: %v", err)
	}
	var jpg bytes.Buffer
	_ = jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 256, 256)), nil)
	if _, err := ValidateAsset(AssetLogo, "logo.jpg", jpg.Bytes()); err != ErrAssetType {
		t.Fatalf("jpeg logo: %v", err)
	}
	if _, err := ValidateAsset(AssetLogo, "huge.png", pngOf(t, 1025, 1025)); err != ErrAssetDimension {
		t.Fatalf("1025px: %v", err)
	}
}

func TestValidateAssetFaviconAndSVG(t *testing.T) {
	if _, err := ValidateAsset(AssetFavicon, "icon.png", pngOf(t, 32, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateAsset(AssetFavicon, "icon.png", pngOf(t, 33, 33)); err != ErrAssetDimension {
		t.Fatalf("33px favicon: %v", err)
	}
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256"><rect width="256" height="256" fill="#92400E"/></svg>`)
	if _, err := ValidateAsset(AssetLogo, "mark.svg", svg); err != nil {
		t.Fatal(err)
	}
	bad := []byte(`<svg viewBox="0 0 256 256"><script>alert(1)</script></svg>`)
	if _, err := ValidateAsset(AssetLogo, "bad.svg", bad); err != ErrAssetSVG {
		t.Fatalf("script svg: %v", err)
	}
}
