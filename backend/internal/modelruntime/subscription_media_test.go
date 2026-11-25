package modelruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/subscriptionauth"
)

const mediaTestPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLttAAAAABJRU5ErkJggg=="

type mediaTestCredentials map[Operation]SubscriptionCredential

func (c mediaTestCredentials) ResolveSubscriptionCredential(_ context.Context, _ Attempt, operation Operation) (SubscriptionCredential, error) {
	credential, ok := c[operation]
	if !ok {
		return credential, ErrSubscriptionAuthUnavailable
	}
	credential.AccessToken = "mock-subscription-token"
	return credential, nil
}

func newMediaTestRuntime(t *testing.T, origin string, credentials mediaTestCredentials) *Bifrost {
	t.Helper()
	guard := func(_ context.Context, target string) error {
		if !strings.HasPrefix(target, origin+"/") {
			return ErrInvalidRequest
		}
		return nil
	}
	b, err := NewBifrost(context.Background(), BifrostConfig{
		Providers: []string{"openai", "gemini"}, Credentials: testCredentials{}, Subscriptions: credentials,
		AllowTestLoopback: true, EndpointGuard: guard, ResourceGuard: guard,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	return b
}

func TestSubscriptionImageEditProtocolsAndPreview(t *testing.T) {
	for _, kind := range []string{subscriptionauth.Codex, subscriptionauth.Gemini, subscriptionauth.Antigravity, subscriptionauth.Grok} {
		t.Run(kind, func(t *testing.T) {
			protocol, path, model := "openai", "/v1/images/edits", "gpt-image-1"
			stream := kind == subscriptionauth.Codex
			switch kind {
			case subscriptionauth.Gemini, subscriptionauth.Antigravity:
				model = "gemini-2.5-flash-image"
				protocol, path = "gemini", "/v1/projects/example/locations/global/publishers/google/models/"+model+":generateContent"
			case subscriptionauth.Grok:
				protocol, model = "xai", "grok-imagine-image-2.0"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != path || r.Header.Get("Authorization") != "Bearer mock-subscription-token" || r.Header.Get("X-Goog-Api-Key") != "" || r.URL.Query().Has("key") {
					t.Error("wrong connection path or authentication")
				}
				w.Header().Set("Content-Type", "application/json")
				switch protocol {
				case "gemini":
					var body struct {
						Contents []struct {
							Parts []struct {
								Text       string `json:"text"`
								InlineData struct {
									Data string `json:"data"`
								} `json:"inlineData"`
							} `json:"parts"`
						} `json:"contents"`
					}
					if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Contents) == 0 || len(body.Contents[0].Parts) < 2 {
						t.Error("missing native Gemini prompt or image")
					} else if body.Contents[0].Parts[0].InlineData.Data != mediaTestPNG && body.Contents[0].Parts[1].InlineData.Data != mediaTestPNG {
						t.Error("uploaded image did not reach Gemini")
					}
					fmt.Fprintf(w, `{"candidates":[{"content":{"role":"model","parts":[{"inlineData":{"mimeType":"image/png","data":"%s"}}]},"finishReason":"STOP"}]}`, mediaTestPNG)
				case "xai":
					var body struct {
						Model string               `json:"model"`
						Image struct{ URL string } `json:"image"`
					}
					if json.NewDecoder(r.Body).Decode(&body) != nil || body.Model != model || body.Image.URL != "data:image/png;base64,"+mediaTestPNG {
						t.Error("missing native Grok edit image")
					}
					fmt.Fprintf(w, `{"data":[{"b64_json":"%s"}]}`, mediaTestPNG)
				default:
					if r.ParseMultipartForm(1<<20) != nil || r.FormValue("model") != model || r.FormValue("prompt") != "edit" ||
						len(r.MultipartForm.File["image"])+len(r.MultipartForm.File["image[]"]) != 1 {
						t.Error("missing OpenAI-compatible multipart edit")
					}
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintf(w, "data: {\"type\":\"image_edit.partial_image\",\"b64_json\":\"%s\",\"partial_image_index\":0}\n\ndata: {\"type\":\"image_edit.completed\",\"b64_json\":\"%s\",\"index\":0}\n\n", mediaTestPNG, mediaTestPNG)
				}
			}))
			defer server.Close()
			b := newMediaTestRuntime(t, server.URL, mediaTestCredentials{ImageEdit: {Protocol: protocol, Endpoint: server.URL + path}})
			r, err := DecodeProtocol("request", ImageEdit, []byte(fmt.Sprintf(`{"model":"public-model","prompt":"edit","image":{"image":"%s"},"stream":%t}`, mediaTestPNG, stream)))
			if err != nil {
				t.Fatal(err)
			}
			r.Async = !stream
			a := testAttempt("", DefaultBilling(ImageEdit).Capability(ImageEdit, stream))
			a.SubscriptionKind, a.SubscriptionAccountID, a.UpstreamModel = kind, "account", model
			events := 0
			out, err := b.Execute(context.Background(), a, r, func(_ context.Context, event Event) error {
				events++
				if events == 1 && event.Usage != nil {
					t.Error("preview was metered as final output")
				}
				return nil
			})
			if err != nil || out.State != "completed" || out.Cost != nil || !out.Usage.Reliable ||
				len(out.Usage.Measurements) != 1 || !strings.HasPrefix(out.Usage.Measurements[0].Quantity, "1.") ||
				!strings.Contains(string(out.Output), mediaTestPNG) || stream && events != 2 {
				t.Fatalf("missing verified and retained image, state=%s usage=%+v events=%d err=%v", out.State, out.Usage, events, err)
			}
		})
	}
}

func TestGrokVideoUsesVerifiedDurationAndPrivateBytes(t *testing.T) {
	clip := []byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'm', 'p', '4', '2', 0, 0, 0, 1, 'i', 's', 'o', 'm', 'm', 'p', '4', '2'}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/clip" {
			if r.Header.Get("Authorization") != "" {
				t.Error("account token reached the result host")
			}
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write(clip)
			return
		}
		if r.Header.Get("Authorization") != "Bearer mock-subscription-token" {
			t.Error("missing original subscription authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/videos/generations":
			var body struct{ Duration int }
			if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&body) != nil || body.Duration != 6 {
				t.Error("platform seconds were not translated to native duration")
			}
			fmt.Fprint(w, `{"request_id":"native-task"}`)
		case "/v1/videos/native-task":
			fmt.Fprintf(w, `{"status":"done","video":{"url":"http://%s/clip","duration":5,"width":1280,"height":720}}`, r.Host)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	b := newMediaTestRuntime(t, server.URL, mediaTestCredentials{
		Video:         {Protocol: "xai", Endpoint: server.URL + "/v1/videos/generations"},
		VideoRetrieve: {Protocol: "xai", Endpoint: server.URL + "/v1/videos/native-task"},
	})
	r, err := DecodeProtocol("request", Video, []byte(`{"model":"public-model","prompt":"blue waves","seconds":"6","size":"1280x720"}`))
	if err != nil {
		t.Fatal(err)
	}
	profile := DefaultBilling(Video)
	profile.Output[0].Variants = []BillingVariant{{ID: "hd", Name: "HD", Size: "1280x720"}}
	a := testAttempt("", profile.Capability(Video, false))
	a.SubscriptionKind, a.SubscriptionAccountID = subscriptionauth.Grok, "account"
	started, err := b.Execute(context.Background(), a, r, nil)
	if err != nil || started.State != "submitted" || started.TaskID != "native-task" || started.Usage.Reliable {
		t.Fatalf("unexpected video submission: %+v, %v", started, err)
	}
	a.TaskID, r.Operation = started.TaskID, VideoRetrieve
	out, err := b.Execute(context.Background(), a, r, nil)
	if err != nil || !out.Usage.Reliable || string(out.Binary) != string(clip) || strings.Contains(string(out.Output), "/clip") ||
		len(out.Usage.Measurements) != 2 || !strings.HasPrefix(out.Usage.Measurements[0].Quantity, "5.") || out.Usage.Measurements[0].Component != "output_video_seconds:hd" {
		t.Fatalf("video did not meter the actual clip: %+v, %v", out.Usage, err)
	}
}
