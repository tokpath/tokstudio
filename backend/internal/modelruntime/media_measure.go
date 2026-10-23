package modelruntime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"image"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/maximhq/bifrost/core/network"
	"github.com/maximhq/bifrost/core/schemas"
)

// metadataBytes fetches only bounded, public resource metadata. It never sends
// a provider credential and pins the DNS answer at the actual connection.
func (b *Bifrost) metadataBytes(ctx context.Context, target, byteRange string) ([]byte, error) {
	u, e := url.Parse(target)
	if e != nil || u.User != nil || u.Hostname() == "" || (u.Scheme != "https" && !(b.config.AllowTestLoopback && u.Scheme == "http")) {
		return nil, ErrInvalidRequest
	}
	if b.config.ResourceGuard != nil {
		if e = b.config.ResourceGuard(ctx, target); e != nil {
			return nil, e
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	transport := &http.Transport{DialContext: func(c context.Context, kind, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupIP(c, "ip", host)
		if e != nil || len(ips) == 0 {
			return nil, ErrInvalidRequest
		}
		for _, ip := range ips {
			if !network.IsPublicIP(ip) && !(b.config.AllowTestLoopback && ip.IsLoopback()) {
				return nil, ErrInvalidRequest
			}
		}
		for _, ip := range ips {
			if conn, e := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(c, kind, net.JoinHostPort(ip.String(), port)); e == nil {
				return conn, nil
			}
		}
		return nil, ErrInvalidRequest
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, e := http.NewRequestWithContext(ctx, "GET", target, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Range", byteRange)
	resp, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return nil, ErrUsageUnknown
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

// measurableResponse supplies verified file metadata to the quantity adapter.
// SDK request backfills remain untouched and are not treated as measurements.
func (b *Bifrost) measurableResponse(ctx context.Context, response any, c Capability) any {
	if c.Billing == nil {
		return response
	}
	if needsImageMetadata(*c.Billing) {
		raw, _ := json.Marshal(response)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		data, ok := outputImages(body)
		if ok {
			for _, entry := range data {
				item, _ := entry.(map[string]any)
				if w, h := imageDimensions(item); w > 0 && h > 0 {
					continue
				}
				target, _ := item["url"].(string)
				if raw, e := b.metadataBytes(ctx, target, "bytes=0-4194303"); e == nil {
					if cfg, _, e := image.DecodeConfig(bytes.NewReader(raw)); e == nil && cfg.Width > 0 && cfg.Height > 0 && cfg.Width <= 65536 && cfg.Height <= 65536 {
						item["width"] = json.Number(strconv.Itoa(cfg.Width))
						item["height"] = json.Number(strconv.Itoa(cfg.Height))
					}
				}
			}
			body["data"] = data
			return body
		}
	}
	if video, ok := response.(*schemas.BifrostVideoGenerationResponse); ok && video != nil && video.Status == schemas.VideoStatusCompleted && c.Billing.Has("output", "video", "seconds") {
		if KnownVideoSeconds(video) != nil {
			return video
		}
		total := new(big.Rat)
		if len(video.Videos) == 0 {
			return video
		}
		for _, asset := range video.Videos {
			var duration *big.Rat
			if asset.Base64Data != nil && len(*asset.Base64Data) <= 48<<20 {
				if raw, e := base64.StdEncoding.DecodeString(*asset.Base64Data); e == nil {
					duration = mp4Seconds(raw)
				}
			}
			if duration == nil && asset.URL != nil {
				if raw, e := b.metadataBytes(ctx, *asset.URL, "bytes=0-4194303"); e == nil {
					duration = mp4Seconds(raw)
				}
				if duration == nil {
					if raw, e := b.videoMetadataTail(ctx, *asset.URL); e == nil {
						duration = mp4Seconds(raw)
					}
				}
			}
			if duration == nil {
				return video
			}
			total.Add(total, duration)
		}
		copy := *video
		copy.ExtraFields = video.ExtraFields
		copy.ExtraFields.RawResponse = map[string]any{"status": "completed", "seconds": total.FloatString(18)}
		return &copy
	}
	return response
}

// KnownVideoSeconds reads the native completed task, never SDK .Seconds which
// may have been copied from the original request. Multiple outputs are summed.
func KnownVideoSeconds(response any) *big.Rat {
	raw, e := json.Marshal(response)
	if e != nil {
		return nil
	}
	var body struct {
		Extra struct {
			Raw json.RawMessage `json:"raw_response"`
		} `json:"extra_fields"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return nil
	}
	native := body.Extra.Raw
	var text string
	if json.Unmarshal(native, &text) == nil {
		native = []byte(text)
	}
	var obj map[string]any
	decoder := json.NewDecoder(bytes.NewReader(native))
	decoder.UseNumber()
	if decoder.Decode(&obj) != nil || obj["status"] != "completed" {
		return nil
	}
	if clips, ok := obj["videos"].([]any); ok && len(clips) > 1 {
		total := new(big.Rat)
		for _, clip := range clips {
			item, _ := clip.(map[string]any)
			n, ok := numeric(item["seconds"])
			if !ok {
				n, ok = numeric(item["duration"])
			}
			if !ok {
				return nil
			}
			total.Add(total, n)
		}
		return total
	}
	for _, key := range []string{"seconds", "duration"} {
		if n, ok := numeric(obj[key]); ok && n.Sign() > 0 {
			return n
		}
	}
	return nil
}

func boxBody(raw []byte, kind string) []byte {
	for pos := 0; pos+8 <= len(raw); {
		n := int64(binary.BigEndian.Uint32(raw[pos : pos+4]))
		header := 8
		if n == 1 {
			if pos+16 > len(raw) {
				return nil
			}
			wide := binary.BigEndian.Uint64(raw[pos+8 : pos+16])
			if wide > 1<<40 {
				return nil
			}
			n = int64(wide)
			header = 16
		}
		if n == 0 {
			n = int64(len(raw) - pos)
		}
		if n < int64(header) || n > int64(len(raw)-pos) {
			return nil
		}
		if string(raw[pos+4:pos+8]) == kind {
			return raw[pos+header : pos+int(n)]
		}
		pos += int(n)
	}
	return nil
}
func mp4DisplaySize(raw []byte) string {
	tkhd := boxBody(boxBody(boxBody(raw, "moov"), "trak"), "tkhd")
	if len(tkhd) < 84 || (tkhd[0] != 0 && tkhd[0] != 1) {
		return ""
	}
	widthAt := 76
	if tkhd[0] == 1 {
		widthAt = 80
	}
	if len(tkhd) < widthAt+8 {
		return ""
	}
	w := binary.BigEndian.Uint32(tkhd[widthAt:widthAt+4]) >> 16
	h := binary.BigEndian.Uint32(tkhd[widthAt+4:widthAt+8]) >> 16
	if w == 0 || h == 0 || w > 65536 || h > 65536 {
		return ""
	}
	return strconv.FormatUint(uint64(w), 10) + "x" + strconv.FormatUint(uint64(h), 10)
}
func mp4Seconds(raw []byte) *big.Rat {
	for pos := 0; pos+8 <= len(raw); {
		n := int64(binary.BigEndian.Uint32(raw[pos : pos+4]))
		header := 8
		if n == 1 {
			if pos+16 > len(raw) {
				return nil
			}
			wide := binary.BigEndian.Uint64(raw[pos+8 : pos+16])
			if wide > 1<<40 {
				return nil
			}
			n = int64(wide)
			header = 16
		}
		if n == 0 {
			n = int64(len(raw) - pos)
		}
		if n < int64(header) || n > int64(len(raw)-pos) {
			return nil
		}
		if string(raw[pos+4:pos+8]) == "moov" {
			body := raw[pos+header : pos+int(n)]
			for offset := 0; offset+8 <= len(body); {
				size := int(binary.BigEndian.Uint32(body[offset : offset+4]))
				if size < 8 || size > len(body)-offset {
					return nil
				}
				if string(body[offset+4:offset+8]) == "mvhd" {
					v := body[offset+8 : offset+size]
					if len(v) < 20 {
						return nil
					}
					var scale uint32
					var duration uint64
					switch v[0] {
					case 0:
						scale = binary.BigEndian.Uint32(v[12:16])
						duration = uint64(binary.BigEndian.Uint32(v[16:20]))
					case 1:
						if len(v) < 32 {
							return nil
						}
						scale = binary.BigEndian.Uint32(v[20:24])
						duration = binary.BigEndian.Uint64(v[24:32])
					default:
						return nil
					}
					if scale == 0 || duration == 0 || duration > uint64(scale)*86400 {
						return nil
					}
					return new(big.Rat).SetFrac(new(big.Int).SetUint64(duration), big.NewInt(int64(scale)))
				}
				offset += size
			}
			return nil
		}
		pos += int(n)
	}
	return nil
}

// Follow a declared top-level atom boundary. Never search an mdat payload for
// coincidentally named metadata or treat a partial file as the complete clip.
func (b *Bifrost) videoMetadataTail(ctx context.Context, target string) ([]byte, error) {
	head, e := b.metadataBytes(ctx, target, "bytes=0-4194303")
	if e != nil {
		return nil, e
	}
	for pos := 0; pos+8 <= len(head); {
		size := uint64(binary.BigEndian.Uint32(head[pos : pos+4]))
		header := 8
		if size == 1 {
			if pos+16 > len(head) {
				return nil, ErrUsageUnknown
			}
			size = binary.BigEndian.Uint64(head[pos+8 : pos+16])
			header = 16
		}
		if size < uint64(header) || size > 1<<40 {
			return nil, ErrUsageUnknown
		}
		next := uint64(pos) + size
		if next > uint64(len(head)) {
			start := strconv.FormatUint(next, 10)
			end := strconv.FormatUint(next+4194303, 10)
			return b.metadataBytes(ctx, target, "bytes="+start+"-"+end)
		}
		pos = int(next)
	}
	return nil, ErrUsageUnknown
}

func needsImageMetadata(profile BillingProfile) bool {
	for _, item := range profile.Output {
		if item.Modality != "image" {
			continue
		}
		if item.Template == "megapixels" {
			return true
		}
		for _, variant := range item.Variants {
			if variant.Size != "" {
				return true
			}
		}
	}
	return false
}
