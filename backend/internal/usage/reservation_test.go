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

func TestImageInsideChatReservesTheInputBound(t *testing.T) {
	q := quote(catalog.ComponentRate{Key: "input", Unit: "token", Per: "1", Rate: "0.001"})
	q.Bounds["input"] = "1000"
	r := modelruntime.Request{Operation: modelruntime.Chat, Input: []byte(`[{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"https://example.test/a.png"}}]}]`)}
	if got := reservationQuantities(q, r)["input"]; got != "1000" {
		t.Fatalf("image input reservation = %s, want the registered bound", got)
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

func TestImageCountUsesTheLargerKnownField(t *testing.T) {
	q := quote(catalog.ComponentRate{Key: "pictures", Unit: "image", Per: "1", Rate: "1"})
	q.Bounds["pictures"] = "8"
	r := modelruntime.Request{Operation: modelruntime.Image, Params: []byte(`{"n":1,"parameters":{"sampleCount":4},"imageGenerationConfig":{"numberOfImages":2}}`)}
	if got := reservationQuantities(q, r)["pictures"]; got != "4" {
		t.Fatalf("image count = %s, want the larger visible count 4", got)
	}
	nested := modelruntime.Request{Operation: modelruntime.Image, Params: []byte(`{"parameters":{"n":3}}`)}
	if got := reservationQuantities(q, nested)["pictures"]; got != "3" {
		t.Fatalf("nested image count = %s", got)
	}
	open := modelruntime.Request{Operation: modelruntime.Image, Params: []byte(`{"sequential_image_generation":"auto"}`)}
	if got := reservationQuantities(q, open)["pictures"]; got != "8" {
		t.Fatalf("open-ended image count = %s, want the registered bound", got)
	}
	capped := modelruntime.Request{Operation: modelruntime.Image, Params: []byte(`{"sequential_image_generation":"auto","sequential_image_generation_options":{"max_images":3}}`)}
	if got := reservationQuantities(q, capped)["pictures"]; got != "3" {
		t.Fatalf("max_images = %s", got)
	}
}

func TestDurationUsesTheLargerKnownField(t *testing.T) {
	q := quote(catalog.ComponentRate{Key: "duration", Unit: "second", Per: "1", Rate: "1"})
	q.Bounds["duration"] = "60"
	r := modelruntime.Request{Operation: modelruntime.Video, Params: []byte(`{"seconds":"4","parameters":{"durationSeconds":8},"video":{"duration":"5s"}}`)}
	if got := reservationQuantities(q, r)["duration"]; got != "8" {
		t.Fatalf("duration = %s, want the larger visible duration 8", got)
	}
	frames := modelruntime.Request{Operation: modelruntime.Video, Params: []byte(`{"num_frames":24,"fps":8}`)}
	if got := reservationQuantities(q, frames)["duration"]; got != "3" {
		t.Fatalf("frame duration = %s, want 3 seconds", got)
	}
	unknownFrames := modelruntime.Request{Operation: modelruntime.Video, Params: []byte(`{"video_length":25}`)}
	if got := reservationQuantities(q, unknownFrames)["duration"]; got != "60" {
		t.Fatalf("frames without a rate = %s, want the registered bound", got)
	}
	several := modelruntime.Request{Operation: modelruntime.Video, Params: []byte(`{"duration":4,"parameters":{"sampleCount":2}}`)}
	if got := reservationQuantities(q, several)["duration"]; got != "8" {
		t.Fatalf("repeated video duration = %s, want 4 * 2", got)
	}
	sound := modelruntime.Request{Operation: modelruntime.Speech, Params: []byte(`{"text":"boom"}`)}
	if got := reservationQuantities(q, sound)["duration"]; got != "60" {
		t.Fatalf("sound effect without duration = %s, want the registered bound", got)
	}
}

func TestSpeechTextUsesKnownFields(t *testing.T) {
	q := quote(catalog.ComponentRate{Key: "spoken", Unit: "character", Per: "1", Rate: "1"})
	q.Bounds["spoken"] = "500"
	eleven := modelruntime.Request{Operation: modelruntime.Speech, Params: []byte(`{"text":"你好","voice_settings":{"stability":0.5}}`)}
	if got := reservationQuantities(q, eleven)["spoken"]; got != "2" {
		t.Fatalf("elevenlabs text = %s", got)
	}
	longer := modelruntime.Request{Operation: modelruntime.Speech, Params: []byte(`{"text":"hi","ssml":"<speak>hello</speak>"}`)}
	if got := reservationQuantities(q, longer)["spoken"]; got != "20" {
		t.Fatalf("longer speech source = %s, want 20", got)
	}
	turns := modelruntime.Request{Operation: modelruntime.Speech, Params: []byte(`{"multiSpeakerMarkup":{"turns":[{"speaker":"A","text":"ab"},{"speaker":"B","text":"cd"}]}}`)}
	if got := reservationQuantities(q, turns)["spoken"]; got != "4" {
		t.Fatalf("speaker turns = %s, want 4", got)
	}
	missing := modelruntime.Request{Operation: modelruntime.Speech, Params: []byte(`{"voice":"alloy"}`)}
	if got := reservationQuantities(q, missing)["spoken"]; got != "500" {
		t.Fatalf("missing speech text = %s, want the registered bound", got)
	}
}

func TestFinishedVideoLookupReservesNothing(t *testing.T) {
	q := quote(catalog.ComponentRate{Key: "duration", Unit: "second", Per: "1", Rate: "1"})
	r := modelruntime.Request{Operation: modelruntime.VideoRetrieve, PublicModel: "m"}
	if got := reservationQuantities(q, r)["duration"]; got != "0" {
		t.Fatalf("retrieve reservation = %s", got)
	}
}
