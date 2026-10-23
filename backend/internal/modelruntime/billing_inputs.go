package modelruntime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"math/big"
	"strings"
	"unicode/utf8"
)

type inputSources struct {
	text                 []string
	images, audio, video []string
}

func sources(r Request) inputSources {
	var out inputSources
	var root any
	_ = json.Unmarshal(r.Input, &root)
	str := func(v any) string { s, _ := v.(string); return s }
	mediaURL := func(v any) string {
		if s, ok := v.(string); ok {
			return s
		}
		m, _ := v.(map[string]any)
		return str(m["url"])
	}
	content := func(v any) {
		if text, ok := v.(string); ok {
			out.text = append(out.text, text)
			return
		}
		parts, _ := v.([]any)
		for _, raw := range parts {
			part, _ := raw.(map[string]any)
			switch str(part["type"]) {
			case "text", "input_text", "output_text":
				out.text = append(out.text, str(part["text"]))
			case "image_url", "input_image":
				out.images = append(out.images, mediaURL(part["image_url"]))
			case "input_audio":
				audio, _ := part["input_audio"].(map[string]any)
				out.audio = append(out.audio, str(audio["data"]))
			case "video_url", "input_video":
				out.video = append(out.video, mediaURL(part["video_url"]))
			case "file", "input_file":
				file, _ := part["file"].(map[string]any)
				if file == nil {
					file = part
				}
				value := str(file["file_data"])
				if value == "" {
					value = str(file["file_url"])
				}
				mime := str(file["file_type"])
				switch {
				case strings.HasPrefix(mime, "image/"):
					out.images = append(out.images, value)
				case strings.HasPrefix(mime, "audio/"):
					out.audio = append(out.audio, value)
				case strings.HasPrefix(mime, "video/"):
					out.video = append(out.video, value)
				}
			}
		}
	}
	switch r.Operation {
	case Chat, Responses:
		messages, _ := root.([]any)
		for _, raw := range messages {
			message, _ := raw.(map[string]any)
			content(message["content"])
		}
	case Embedding:
		if text, ok := root.(string); ok {
			out.text = append(out.text, text)
		} else if items, ok := root.([]any); ok {
			for _, raw := range items {
				if text, ok := raw.(string); ok {
					out.text = append(out.text, text)
				}
			}
		}
	case Image, Video:
		input, _ := root.(map[string]any)
		out.text = append(out.text, str(input["prompt"]))
		if value := str(input["input_reference"]); value != "" {
			out.images = append(out.images, value)
		}
		if value := str(input["video_uri"]); value != "" {
			out.video = append(out.video, value)
		}
	case Speech:
		input, _ := root.(map[string]any)
		out.text = append(out.text, str(input["input"]))
	case Transcription:
		input, _ := root.(map[string]any)
		out.audio = append(out.audio, str(input["file"]))
	}
	var params map[string]any
	_ = json.Unmarshal(r.Params, &params)
	if r.Operation == Image {
		if values, ok := params["input_images"].([]any); ok {
			for _, value := range values {
				out.images = append(out.images, str(value))
			}
		}
	}
	return out
}
func (s inputSources) characters() int64 {
	var n int64
	for _, text := range s.text {
		n += int64(utf8.RuneCountInString(text))
	}
	return n
}
func (s inputSources) kinds() []string {
	out := []string{}
	if len(s.text) > 0 {
		out = append(out, "text")
	}
	if len(s.images) > 0 {
		out = append(out, "image")
	}
	if len(s.audio) > 0 {
		out = append(out, "audio")
	}
	if len(s.video) > 0 {
		out = append(out, "video")
	}
	return out
}
func assetBytes(value string) []byte {
	if len(value) > 48<<20 {
		return nil
	}
	if strings.HasPrefix(value, "data:") {
		index := strings.Index(value, ",")
		if index < 0 || !strings.HasSuffix(value[:index], ";base64") {
			return nil
		}
		value = value[index+1:]
	}
	raw, _ := base64.StdEncoding.DecodeString(value)
	return raw
}
func (b *Bifrost) inputFacts(ctx context.Context, r Request, p BillingProfile) map[string]string {
	facts := map[string]string{}
	layout := sources(r)
	for _, item := range p.Input {
		if item.Template == "tokens" {
			continue
		}
		if item.Template == "characters" {
			facts[item.Key("input")] = big.NewInt(layout.characters()).String()
			continue
		}
		var assets []string
		switch item.Modality {
		case "image":
			assets = layout.images
		case "audio":
			assets = layout.audio
		case "video":
			assets = layout.video
		}
		quantities := map[string]*big.Rat{}
		for _, component := range item.Components("input") {
			quantities[component.Key] = new(big.Rat)
		}
		reliable := true
		for _, asset := range assets {
			quantity := big.NewRat(1, 1)
			selector := json.RawMessage(`{}`)
			if item.Template != "images" || len(item.Variants) > 0 {
				raw := assetBytes(asset)
				if raw == nil && b != nil {
					raw, _ = b.metadataBytes(ctx, asset, "bytes=0-4194303")
				}
				switch item.Template {
				case "images", "megapixels":
					cfg, _, e := image.DecodeConfig(bytes.NewReader(raw))
					if e != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 65536 || cfg.Height > 65536 {
						reliable = false
						break
					}
					selector, _ = json.Marshal(map[string]string{"size": new(big.Int).SetInt64(int64(cfg.Width)).String() + "x" + new(big.Int).SetInt64(int64(cfg.Height)).String()})
					if item.Template == "megapixels" {
						quantity = big.NewRat(int64(cfg.Width)*int64(cfg.Height), 1000000)
					}
				case "seconds":
					if item.Modality == "audio" {
						quantity = audioSeconds(raw)
					} else {
						quantity = mp4Seconds(raw)
						if quantity == nil && b != nil && strings.HasPrefix(asset, "https://") {
							tail, _ := b.videoMetadataTail(ctx, asset)
							quantity = mp4Seconds(tail)
						}
					}
					if quantity == nil {
						reliable = false
					} else if item.Modality == "video" && len(item.Variants) > 0 {
						size := mp4DisplaySize(raw)
						if size == "" {
							reliable = false
						} else {
							selector, _ = json.Marshal(map[string]string{"size": size})
						}
					}
				}
			}
			if !reliable {
				break
			}
			selected, e := item.SelectVariant(selector)
			if e != nil {
				reliable = false
				break
			}
			key := item.Key("input")
			if selected != "" {
				key += ":" + selected
			}
			quantities[key].Add(quantities[key], quantity)
		}
		if reliable {
			for key, value := range quantities {
				facts[key] = value.FloatString(18)
			}
		}
	}
	return facts
}
func audioSeconds(raw []byte) *big.Rat {
	if duration := wavSeconds(raw); duration != nil {
		return duration
	}
	// FLAC STREAMINFO has an exact sample count and sample rate.
	if len(raw) >= 42 && string(raw[:4]) == "fLaC" && raw[4]&0x7f == 0 {
		v := raw[18:26]
		rate := uint64(v[0])<<12 | uint64(v[1])<<4 | uint64(v[2]>>4)
		samples := uint64(v[3]&15)<<32 | uint64(v[4])<<24 | uint64(v[5])<<16 | uint64(v[6])<<8 | uint64(v[7])
		if rate > 0 && samples > 0 {
			return new(big.Rat).SetFrac(new(big.Int).SetUint64(samples), new(big.Int).SetUint64(rate))
		}
	}
	return mp3Seconds(raw)
}

func mp3Seconds(raw []byte) *big.Rat {
	pos := 0
	if len(raw) >= 10 && string(raw[:3]) == "ID3" {
		for _, v := range raw[6:10] {
			if v&0x80 != 0 {
				return nil
			}
		}
		size := int(raw[6])<<21 | int(raw[7])<<14 | int(raw[8])<<7 | int(raw[9])
		pos = 10 + size
		if raw[5]&0x10 != 0 {
			pos += 10
		}
	}
	total := new(big.Rat)
	frames := 0
	for pos+4 <= len(raw) {
		if len(raw)-pos == 128 && string(raw[pos:pos+3]) == "TAG" {
			pos = len(raw)
			break
		}
		a, b, c := raw[pos], raw[pos+1], raw[pos+2]
		version := (b >> 3) & 3
		layer := (b >> 1) & 3
		index := int(c >> 4)
		sampleIndex := int((c >> 2) & 3)
		if a != 255 || b&0xe0 != 0xe0 || version == 1 || layer != 1 || index == 0 || index == 15 || sampleIndex == 3 {
			return nil
		}
		rates := []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
		samples, coefficient := 1152, 144000
		rate := []int{44100, 48000, 32000}[sampleIndex]
		if version != 3 {
			rates = []int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
			samples, coefficient = 576, 72000
			rate /= 2
			if version == 0 {
				rate /= 2
			}
		}
		size := coefficient*rates[index]/rate + int((c>>1)&1)
		if size < 4 || size > len(raw)-pos {
			return nil
		}
		total.Add(total, big.NewRat(int64(samples), int64(rate)))
		frames++
		pos += size
	}
	if frames == 0 || pos != len(raw) {
		return nil
	}
	return total
}
