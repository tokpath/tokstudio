package usage

import (
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"math/big"
	"testing"
)

func quote(components ...catalog.ComponentRate) catalog.ModelQuote {
	cap := modelruntime.DefaultBilling(modelruntime.Chat).Capability(modelruntime.Chat, false)
	return catalog.ModelQuote{Billing: cap, Retail: catalog.ResolvedPrice{Components: components, FactorNumerator: "1", FactorDenominator: "1"}, Bounds: cap.BillableBounds}
}
func TestReservationUsesCanonicalPublicTemplate(t *testing.T) {
	for _, sample := range []struct {
		op                        modelruntime.Operation
		template, body, key, want string
	}{
		{modelruntime.Chat, "tokens", `{"model":"m","messages":[{"role":"user","content":"hello"}],"max_completion_tokens":100,"n":3,"tools":[{"max_tokens":90000}]}`, "output", "300"},
		{modelruntime.Image, "images", `{"model":"m","prompt":"a dot","n":2,"tool":{"n":99}}`, "output_image_images", "2"},
		{modelruntime.Image, "megapixels", `{"model":"m","prompt":"a dot","n":2,"size":"1024x512"}`, "output_image_megapixels", "1.048576"},
		{modelruntime.Speech, "characters", `{"model":"m","input":"你好， A!","instructions":"ignore this"}`, "input_text_characters", "6"},
		{modelruntime.Video, "seconds", `{"model":"m","prompt":"a dot","seconds":"4.5","n":2,"tool":{"duration":9000}}`, "output_video_seconds", "9"},
	} {
		t.Run(sample.template+string(sample.op), func(t *testing.T) {
			r, e := modelruntime.DecodeProtocol("req", sample.op, []byte(sample.body))
			if e != nil {
				t.Fatal(e)
			}
			p := modelruntime.DefaultBilling(sample.op)
			if sample.template == "megapixels" {
				p.Output[0].Template = "megapixels"
			}
			c := p.Capability(sample.op, false)
			q := catalog.ModelQuote{Billing: c}
			got := reservationQuantities(q, r)[sample.key]
			actual, ok := new(big.Rat).SetString(got)
			want, _ := new(big.Rat).SetString(sample.want)
			if !ok || actual.Cmp(want) != 0 {
				t.Fatalf("%s != %s", got, sample.want)
			}
		})
	}
}
func TestReservationUsesFallbackForUnknownMediaAndIgnoresNestedAliases(t *testing.T) {
	c := modelruntime.DefaultBilling(modelruntime.Chat).Capability(modelruntime.Chat, false)
	r := modelruntime.Request{Operation: modelruntime.Chat, Input: []byte(`[]`), InputTypes: []string{"image"}, Params: []byte(`{"tools":[{"max_tokens":999999}]}`)}
	got := reservationQuantities(catalog.ModelQuote{Billing: c}, r)
	if got["input"] != "131072.000000000000000000" || got["output"] != "4096.000000000000000000" {
		t.Fatal(got)
	}
}
func TestReservationOnlyOccupiesSelectedSpecification(t *testing.T) {
	c := modelruntime.BillingProfile{Output: []modelruntime.BillingItem{{Modality: "image", Template: "images", Variants: []modelruntime.BillingVariant{{ID: "small", Name: "Small", Size: "512x512"}, {ID: "large", Name: "Large", Size: "1024x1024"}}}}}.Capability(modelruntime.Image, false)
	r := modelruntime.Request{Operation: modelruntime.Image, Params: []byte(`{"size":"512x512","n":3}`)}
	got := reservationQuantities(catalog.ModelQuote{Billing: c}, r)
	if got["output_image_images:large"] != "0" || got["output_image_images:small"] != "3.000000000000000000" || got["output_image_images"] != "0" {
		t.Fatal(got)
	}
	r.Operation = modelruntime.VideoRetrieve
	for _, value := range reservationQuantities(catalog.ModelQuote{Billing: c}, r) {
		if value != "0" {
			t.Fatal("retrieval reserved funds")
		}
	}
}
