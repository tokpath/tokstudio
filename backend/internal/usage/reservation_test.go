package usage

import (
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

func quote(components ...catalog.ComponentRate) catalog.ModelQuote {
	bounds := map[string]string{}
	for _, component := range components {
		bounds[component.Key] = "1000"
	}
	return catalog.ModelQuote{Retail: catalog.ResolvedPrice{Components: components, FactorNumerator: "1", FactorDenominator: "1"}, Bounds: bounds}
}

func TestTextReservationUsesTheLargerOutputCeiling(t *testing.T) {
	q := quote(catalog.ComponentRate{Key: "input", Unit: "token", Per: "1", Rate: "0.001"}, catalog.ComponentRate{Key: "output", Unit: "token", Per: "1", Rate: "0.001"})
	r := modelruntime.Request{Operation: modelruntime.Chat, Input: []byte(`[{"role":"user","content":"hello"}]`), Params: []byte(`{"max_tokens":40,"max_completion_tokens":100,"reasoning":{"max_tokens":20}}`)}
	got := reservationQuantities(q, r)
	if got["output"] != "100" {
		t.Fatalf("output ceiling = %s, want the larger declared limit 100", got["output"])
	}
	if got["input"] == "1000" {
		t.Fatal("input was reserved at the registered maximum instead of this request")
	}
}

func TestMissingOutputCeilingReservesTheRegisteredBound(t *testing.T) {
	q := quote(catalog.ComponentRate{Key: "output", Unit: "token", Per: "1", Rate: "1"})
	q.Bounds["output"] = "60"
	r, err := modelruntime.DecodeProtocol("incoming", modelruntime.Responses, []byte(`{"model":"m","input":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := reservationQuantities(q, r)["output"]; got != "60" {
		t.Fatalf("output = %s, want registered bound 60", got)
	}
}

func TestImageSpeechAndVideoUseVisibleQuantities(t *testing.T) {
	image := quote(catalog.ComponentRate{Key: "pictures", Unit: "image", Per: "1", Rate: "1"})
	image.Bounds["pictures"] = "8"
	one, err := modelruntime.DecodeProtocol("incoming", modelruntime.Image, []byte(`{"model":"m","prompt":"a cat"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := reservationQuantities(image, one)["pictures"]; got != "1" {
		t.Fatalf("default image count = %s", got)
	}
	many, err := modelruntime.DecodeProtocol("incoming", modelruntime.Image, []byte(`{"model":"m","prompt":"a cat","n":3}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := reservationQuantities(image, many)["pictures"]; got != "3" {
		t.Fatalf("image count = %s", got)
	}
	speech := quote(catalog.ComponentRate{Key: "spoken", Unit: "character", Per: "1", Rate: "1"})
	said, err := modelruntime.DecodeProtocol("incoming", modelruntime.Speech, []byte(`{"model":"m","input":"你好"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := reservationQuantities(speech, said)["spoken"]; got != "2" {
		t.Fatalf("speech characters = %s", got)
	}
	video := quote(catalog.ComponentRate{Key: "duration", Unit: "second", Per: "1", Rate: "1"})
	video.Bounds["duration"] = "60"
	short, err := modelruntime.DecodeProtocol("incoming", modelruntime.Video, []byte(`{"model":"m","prompt":"a dot","seconds":"4"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := reservationQuantities(video, short)["duration"]; got != "4" {
		t.Fatalf("video seconds = %s", got)
	}
	open, err := modelruntime.DecodeProtocol("incoming", modelruntime.Video, []byte(`{"model":"m","prompt":"a dot"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := reservationQuantities(video, open)["duration"]; got != "60" {
		t.Fatalf("unknown video duration = %s, want the registered bound", got)
	}
}

func TestRepeatedSamplesMultiplyTokenReservation(t *testing.T) {
	q := quote(catalog.ComponentRate{Key: "output", Unit: "token", Per: "1", Rate: "1"})
	r, err := modelruntime.DecodeProtocol("incoming", modelruntime.Chat, []byte(`{"model":"m","messages":[{"role":"user","content":"hello"}],"max_completion_tokens":10,"n":3}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := reservationQuantities(q, r)["output"]; got != "30" {
		t.Fatalf("output = %s, want 10 * 3", got)
	}
}

func TestFinishedVideoLookupReservesNothing(t *testing.T) {
	q := quote(catalog.ComponentRate{Key: "duration", Unit: "second", Per: "1", Rate: "1"})
	r := modelruntime.Request{Operation: modelruntime.VideoRetrieve, PublicModel: "m"}
	if got := reservationQuantities(q, r)["duration"]; got != "0" {
		t.Fatalf("retrieve reservation = %s", got)
	}
}
