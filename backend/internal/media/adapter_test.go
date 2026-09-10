package media

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemoteAdapterReady(t *testing.T) {
	empty := RemoteAdapter{NameValue: "ark", BaseURL: "https://example.test"}
	if empty.Ready() {
		t.Fatal("key missing should not be ready")
	}
	if (RemoteAdapter{APIKey: "k"}).Ready() {
		t.Fatal("url missing should not be ready")
	}
	if !(RemoteAdapter{BaseURL: "https://example.test", APIKey: "k"}).Ready() {
		t.Fatal("url+key should be ready")
	}
}

func TestRemoteAdapterArkCreateGet(t *testing.T) {
	var creates int
	files := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("ark-mp4"))
	}))
	defer files.Close()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ark-secret" {
			t.Fatalf("auth %s", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/contents/generations/tasks":
			creates++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["model"] != "seedance-1-0-ark" {
				t.Fatalf("model %+v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"cgt-1","status":"queued"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/contents/generations/tasks/cgt-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"cgt-1","status":"succeeded","duration":5,"content":{"video_url":"` + files.URL + `/clip.mp4"}}`))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer up.Close()
	a := RemoteAdapter{NameValue: "ark", BaseURL: up.URL, APIKey: "ark-secret", HTTP: up.Client()}
	created, err := a.Create(t.Context(), SubmitInput{JobID: "vid_1", Kind: KindVideo, Model: "seedance-1-0-ark", Prompt: "cat", Duration: 5})
	if err != nil || created.UpstreamID != "cgt-1" || created.Status != StatusInProgress {
		t.Fatalf("create %+v %v", created, err)
	}
	got, err := a.Get(t.Context(), "cgt-1")
	if err != nil || got.Status != StatusCompleted || string(got.Content) != "ark-mp4" {
		t.Fatalf("get %+v %v", got, err)
	}
	if creates != 1 {
		t.Fatalf("creates %d", creates)
	}
}

func TestRemoteAdapterOpenRouterCreateGet(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer or-secret" {
			http.Error(w, "auth", 401)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/videos":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"or_1","status":"pending"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/videos/or_1":
			_, _ = w.Write([]byte(`{"id":"or_1","status":"completed"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/videos/or_1/content":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("or-mp4"))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer up.Close()
	a := RemoteAdapter{NameValue: "openrouter", BaseURL: up.URL, APIKey: "or-secret", HTTP: up.Client()}
	created, err := a.Create(t.Context(), SubmitInput{JobID: "vid_2", Kind: KindVideo, Model: "bytedance/seedance-1.0", Prompt: "dog"})
	if err != nil || created.UpstreamID != "or_1" {
		t.Fatalf("create %+v %v", created, err)
	}
	got, err := a.Get(t.Context(), "or_1")
	if err != nil || string(got.Content) != "or-mp4" {
		t.Fatalf("get %+v %v", got, err)
	}
}
