package modelruntime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"github.com/maximhq/bifrost/core/schemas"
	"image"
	"image/png"
	"math/big"
	"testing"
)

func picture(t *testing.T, w, h int) string {
	t.Helper()
	var buf bytes.Buffer
	if e := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); e != nil {
		t.Fatal(e)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}
func qmap(u Usage) map[string]*big.Rat {
	out := map[string]*big.Rat{}
	for _, m := range u.Measurements {
		out[m.Component], _ = new(big.Rat).SetString(m.Quantity)
	}
	return out
}
func TestBillingDirectionsRejectOverlappingInputAndOutput(t *testing.T) {
	cases := []struct {
		profile BillingProfile
		valid   bool
	}{
		{BillingProfile{Input: []BillingItem{{Modality: "all", Template: "tokens"}, {Modality: "image", Template: "images"}}}, false},
		{BillingProfile{Input: []BillingItem{{Modality: "image", Template: "images"}, {Modality: "image", Template: "megapixels"}}}, false},
		{BillingProfile{Input: []BillingItem{{Modality: "text", Template: "tokens"}, {Modality: "image", Template: "megapixels"}}, Output: []BillingItem{{Modality: "text", Template: "tokens"}}}, true},
		{BillingProfile{Output: []BillingItem{{Modality: "audio", Template: "images"}}}, false},
	}
	for _, c := range cases {
		if got := c.profile.Valid(Chat); got != c.valid {
			t.Fatalf("%+v valid=%v", c.profile, got)
		}
	}
}
func TestCombinedInputUsesTextTokensAndActualImageAreaWithoutOverlap(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"model": "public", "messages": []any{map[string]any{"role": "user", "content": []any{map[string]string{"type": "text", "text": "describe"}, map[string]any{"type": "image_url", "image_url": map[string]string{"url": picture(t, 2, 3)}}}}}})
	r, e := DecodeProtocol("req", Chat, raw)
	if e != nil {
		t.Fatal(e)
	}
	profile := BillingProfile{Input: []BillingItem{{Modality: "text", Template: "tokens"}, {Modality: "image", Template: "megapixels"}}, Output: []BillingItem{{Modality: "text", Template: "tokens"}}}
	c := profile.Capability(Chat, false)
	response := &schemas.BifrostChatResponse{Usage: &schemas.BifrostLLMUsage{PromptTokens: 30, CompletionTokens: 5, TotalTokens: 35, PromptTokensDetails: &schemas.ChatPromptTokensDetails{TextTokens: 10, ImageTokens: 20}}}
	got := extractBilledUsage(response, c, r)
	if !got.Reliable {
		t.Fatal(got)
	}
	values := qmap(got)
	if values["input_text_tokens"].Cmp(big.NewRat(10, 1)) != 0 || values["input_image_megapixels"].Cmp(big.NewRat(6, 1000000)) != 0 || values["output_text_tokens"].Cmp(big.NewRat(5, 1)) != 0 {
		t.Fatal(got)
	}
	response.Usage.PromptTokensDetails = nil
	if got = extractBilledUsage(response, c, r); got.Reliable {
		t.Fatal("aggregate input was charged again as text beside image area", got)
	}
	aggregate := DefaultBilling(Chat).Capability(Chat, false)
	if got = extractBilledUsage(response, aggregate, r); !got.Reliable || qmap(got)["input"].Cmp(big.NewRat(30, 1)) != 0 {
		t.Fatal("aggregate pricing must remain available", got)
	}
}
func TestInputImagesCountEverySubmittedImageAndRejectUnknownArea(t *testing.T) {
	r := Request{Operation: Image, Input: json.RawMessage(`{"prompt":"hi"}`), Params: json.RawMessage(`{"input_images":["https://example.test/a.png","https://example.test/a.png"]}`)}
	profile := BillingProfile{Input: []BillingItem{{Modality: "image", Template: "images"}}, Output: []BillingItem{{Modality: "image", Template: "images"}}}
	response := map[string]any{"data": []any{map[string]string{"url": "https://example.test/out.png"}}}
	got := extractBilledUsage(response, profile.Capability(Image, false), r)
	if !got.Reliable || qmap(got)["input_image_images"].Cmp(big.NewRat(2, 1)) != 0 {
		t.Fatal(got)
	}
	profile.Input[0].Template = "megapixels"
	if got = extractBilledUsage(response, profile.Capability(Image, false), r); got.Reliable {
		t.Fatal("unverified input size became chargeable", got)
	}
}
func TestTranscriptionBillsProcessedInputSecondsAndOutputCharacters(t *testing.T) {
	r := Request{Operation: Transcription, Input: json.RawMessage(`{"file":"not-used-for-provider-reported-duration"}`)}
	p := BillingProfile{Input: []BillingItem{{Modality: "audio", Template: "seconds"}}, Output: []BillingItem{{Modality: "text", Template: "characters"}}}
	seconds := 1.25
	response := &schemas.BifrostTranscriptionResponse{Text: "你好!", Usage: &schemas.TranscriptionUsage{Type: "duration", Seconds: &seconds}}
	got := extractBilledUsage(response, p.Capability(Transcription, false), r)
	if !got.Reliable || qmap(got)["input_audio_seconds"].Cmp(big.NewRat(5, 4)) != 0 || qmap(got)["output_text_characters"].Cmp(big.NewRat(3, 1)) != 0 {
		t.Fatal(got)
	}
}

func TestStreamCharactersAreConfirmedWithoutInventingTokenUsage(t *testing.T) {
	p := BillingProfile{Output: []BillingItem{{Modality: "text", Template: "characters"}}}
	c := p.Capability(Chat, true)
	var chat schemas.BifrostChatResponse
	if e := json.Unmarshal([]byte(`{"choices":[{"index":0,"delta":{"content":"你好!"}}]}`), &chat); e != nil {
		t.Fatal(e)
	}
	chunk := schemas.BifrostStreamChunk{BifrostChatResponse: &chat}
	s := billingStream{}
	got := s.measure(context.Background(), nil, &chunk, c, Request{Operation: Chat})
	if !got.Reliable || qmap(got)["output_text_characters"].Cmp(big.NewRat(3, 1)) != 0 {
		t.Fatal(got)
	}
	// An SDK zero usage envelope must not turn this partial stream into a token bill.
	tokenProfile := DefaultBilling(Chat).Capability(Chat, true)
	if got = s.measure(context.Background(), nil, &chunk, tokenProfile, Request{Operation: Chat}); got.Reliable {
		t.Fatal("invented token evidence", got)
	}
}
func TestNoOutputImageIsZeroOnlyInCompleteEnvelope(t *testing.T) {
	p := BillingProfile{Output: []BillingItem{{Modality: "image", Template: "images"}}}
	c := p.Capability(Chat, false)
	complete := map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "hello"}}}}
	if got := extractBilledUsage(complete, c, Request{Operation: Chat}); !got.Reliable || qmap(got)["output_image_images"].Sign() != 0 {
		t.Fatal(got)
	}
	if got := extractBilledUsage(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": "hello"}}}}, c, Request{Operation: Chat}); got.Reliable {
		t.Fatal("partial output was treated as complete", got)
	}
}
func TestMP4DurationFollowsAtomBoundaries(t *testing.T) {
	box := func(kind string, body []byte) []byte {
		out := make([]byte, 8+len(body))
		binary.BigEndian.PutUint32(out, uint32(len(out)))
		copy(out[4:], kind)
		copy(out[8:], body)
		return out
	}
	mvhd := make([]byte, 20)
	binary.BigEndian.PutUint32(mvhd[12:16], 1000)
	binary.BigEndian.PutUint32(mvhd[16:20], 1250)
	moov := box("moov", box("mvhd", mvhd))
	if got := mp4Seconds(append(box("ftyp", nil), moov...)); got == nil || got.Cmp(big.NewRat(5, 4)) != 0 {
		t.Fatal(got)
	}
	if got := mp4Seconds(box("mdat", moov)); got != nil {
		t.Fatal("trusted payload as metadata", got)
	}
	if got := mp4Seconds(moov[:len(moov)-1]); got != nil {
		t.Fatal("trusted truncated metadata", got)
	}
}

func TestAggregateAudioDurationCannotBeChargedAsOutput(t *testing.T) {
	profile := BillingProfile{Output: []BillingItem{{Modality: "audio", Template: "seconds"}}}
	body := map[string]any{"choices": []any{map[string]any{"message": map[string]any{"audio": map[string]any{"data": "unmeasurable"}}}}, "extra_fields": map[string]any{"raw_response": map[string]any{"usage": map[string]any{"audio_seconds": 10}}}}
	if got := extractBilledUsage(body, profile.Capability(Chat, false), Request{Operation: Chat}); got.Reliable {
		t.Fatal("possibly input-inclusive duration charged as output", got)
	}
	images := BillingProfile{Output: []BillingItem{{Modality: "image", Template: "images"}}}
	bad := map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{}}}}}}}
	if got := extractBilledUsage(bad, images.Capability(Chat, false), Request{Operation: Chat}); got.Reliable {
		t.Fatal("malformed declared output treated as no image", got)
	}
}
