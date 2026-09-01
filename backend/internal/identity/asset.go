package identity

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"regexp"
	"strings"
)

const (
	AssetLogo      = "logo"
	AssetLogoDark  = "logo_dark"
	AssetFavicon   = "favicon"
	AssetOG        = "og_image"
	AssetFormMax   = 600 * 1024
	AssetHourLimit = 20
	LogoMaxBytes   = 128 * 1024
	FaviconMax     = 64 * 1024
	OGMaxBytes     = 512 * 1024
	LogoMinSide    = 64
	LogoMaxSide    = 1024
	LogoMaxRatio   = 4.0
)

type AssetSpec struct {
	MaxBytes int
	Kinds    map[string]struct{}
}

var assetSpecs = map[string]AssetSpec{
	AssetLogo:     {MaxBytes: LogoMaxBytes},
	AssetLogoDark: {MaxBytes: LogoMaxBytes},
	AssetFavicon:  {MaxBytes: FaviconMax},
	AssetOG:       {MaxBytes: OGMaxBytes},
}

type DecodedAsset struct {
	Kind        string
	ContentType string
	Ext         string
	Width       int
	Height      int
	SHA256      string
	Size        int
}

var viewBoxRe = regexp.MustCompile(`(?i)viewBox\s*=\s*["']\s*([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)`)

func ValidateAsset(kind, filename string, data []byte) (*DecodedAsset, error) {
	kind = strings.TrimSpace(kind)
	spec, ok := assetSpecs[kind]
	if !ok {
		return nil, ErrAssetKind
	}
	if len(data) == 0 || len(data) > spec.MaxBytes {
		return nil, ErrAssetTooLarge
	}
	decoded, err := inspectAsset(kind, filename, data)
	if err != nil {
		return nil, err
	}
	if err := checkDimensions(kind, decoded.Width, decoded.Height, decoded.ContentType); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	decoded.Kind = kind
	decoded.SHA256 = hex.EncodeToString(sum[:])
	decoded.Size = len(data)
	return decoded, nil
}

func inspectAsset(kind, filename string, data []byte) (*DecodedAsset, error) {
	name := strings.ToLower(filename)
	switch {
	case bytes.HasPrefix(data, []byte("<svg")) || bytes.Contains(data[:min(len(data), 256)], []byte("<svg")):
		if kind != AssetLogo && kind != AssetLogoDark {
			return nil, ErrAssetType
		}
		if svgUnsafe(string(data)) {
			return nil, ErrAssetSVG
		}
		w, h, err := svgViewBox(string(data))
		if err != nil {
			return nil, err
		}
		return &DecodedAsset{ContentType: "image/svg+xml", Ext: "svg", Width: w, Height: h}, nil
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		cfg, err := decodeConfig(data)
		if err != nil {
			return nil, ErrAssetType
		}
		return &DecodedAsset{ContentType: "image/png", Ext: "png", Width: cfg.Width, Height: cfg.Height}, nil
	case bytes.HasPrefix(data, []byte("\xff\xd8")):
		if kind != AssetOG {
			return nil, ErrAssetType
		}
		cfg, err := decodeConfig(data)
		if err != nil {
			return nil, ErrAssetType
		}
		return &DecodedAsset{ContentType: "image/jpeg", Ext: "jpg", Width: cfg.Width, Height: cfg.Height}, nil
	case isICO(data):
		if kind != AssetFavicon {
			return nil, ErrAssetType
		}
		w, h, err := icoSize(data)
		if err != nil {
			return nil, err
		}
		return &DecodedAsset{ContentType: "image/x-icon", Ext: "ico", Width: w, Height: h}, nil
	case isWebP(data):
		if kind == AssetFavicon {
			return nil, ErrAssetType
		}
		w, h, err := webpSize(data)
		if err != nil {
			return nil, ErrAssetType
		}
		return &DecodedAsset{ContentType: "image/webp", Ext: "webp", Width: w, Height: h}, nil
	default:
		_ = name
		return nil, ErrAssetType
	}
}

func checkDimensions(kind string, w, h int, contentType string) error {
	if w <= 0 || h <= 0 {
		return ErrAssetDimension
	}
	switch kind {
	case AssetLogo, AssetLogoDark:
		if contentType == "image/svg+xml" {
			ratio := float64(w) / float64(h)
			if ratio < 1 || ratio > LogoMaxRatio+1e-9 {
				return ErrAssetDimension
			}
			return nil
		}
		short, long := w, h
		if h < w {
			short, long = h, w
		}
		if short < LogoMinSide || long > LogoMaxSide {
			return ErrAssetDimension
		}
		if float64(w)/float64(h) < 1-1e-9 || float64(w)/float64(h) > LogoMaxRatio+1e-9 {
			return ErrAssetDimension
		}
	case AssetFavicon:
		if !((w == 32 && h == 32) || (w == 48 && h == 48)) {
			return ErrAssetDimension
		}
	case AssetOG:
		if w != 1200 || h != 630 {
			return ErrAssetDimension
		}
	}
	return nil
}

func decodeConfig(data []byte) (image.Config, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	return cfg, err
}

func svgUnsafe(raw string) bool {
	low := strings.ToLower(raw)
	banned := []string{"<script", "onclick", "onload", "onerror", "foreignobject", "<!entity", "xlink:href=\"http", "href=\"http", "href='http"}
	for _, item := range banned {
		if strings.Contains(low, item) {
			return true
		}
	}
	return false
}

func svgViewBox(raw string) (int, int, error) {
	match := viewBoxRe.FindStringSubmatch(raw)
	if match == nil {
		return 0, 0, ErrAssetSVG
	}
	var minX, minY, w, h float64
	if _, err := fmt.Sscan(match[1]+" "+match[2]+" "+match[3]+" "+match[4], &minX, &minY, &w, &h); err != nil || w <= 0 || h <= 0 {
		return 0, 0, ErrAssetSVG
	}
	return int(w + 0.5), int(h + 0.5), nil
}

func isICO(data []byte) bool {
	return len(data) >= 6 && data[0] == 0 && data[1] == 0 && data[2] == 1 && data[3] == 0
}

func icoSize(data []byte) (int, int, error) {
	if !isICO(data) {
		return 0, 0, ErrAssetType
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	has32 := false
	w, h := 0, 0
	for i := 0; i < count; i++ {
		off := 6 + i*16
		if off+2 > len(data) {
			return 0, 0, ErrAssetType
		}
		cw, ch := int(data[off]), int(data[off+1])
		if cw == 0 {
			cw = 256
		}
		if ch == 0 {
			ch = 256
		}
		if cw == 32 && ch == 32 {
			has32 = true
			w, h = 32, 32
		}
		if w == 0 {
			w, h = cw, ch
		}
	}
	if !has32 {
		return 0, 0, ErrAssetDimension
	}
	return w, h, nil
}

func isWebP(data []byte) bool {
	return len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP"
}

func webpSize(data []byte) (int, int, error) {
	if !isWebP(data) || len(data) < 30 {
		return 0, 0, ErrAssetType
	}
	if string(data[12:16]) != "VP8X" {
		return 0, 0, ErrAssetType
	}
	w := 1 + int(data[24]) | int(data[25])<<8 | int(data[26])<<16
	h := 1 + int(data[27]) | int(data[28])<<8 | int(data[29])<<16
	return w, h, nil
}

func BrandAssetURL(id string) string {
	return "/v1/public/brand-assets/" + id
}

func BrandAssetObjectKey(brandID, kind, digest, ext string) string {
	return "brand/" + brandID + "/" + kind + "/" + digest + "." + ext
}
