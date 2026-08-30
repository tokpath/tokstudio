package gateway

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBifrostAdapterCallsSandbox(t *testing.T) {
	side := httptest.NewServer(SandboxHandler())
	defer side.Close()
	out, err := BifrostAdapter{BaseURL: side.URL}.Chat(context.Background(), "bifrost-lab", "ok", ChatRequest{
		Model:    "tokenhub/echo-1",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.HTTPStatus != 200 || len(out.Body.Choices) == 0 || !strings.Contains(out.Body.Choices[0].Message.Content, "bifrost:hello") {
		t.Fatalf("sandbox reply: %+v", out)
	}
}

func TestBifrostAdapterWithoutURL(t *testing.T) {
	out, err := BifrostAdapter{}.Chat(context.Background(), "x", "ok", ChatRequest{})
	if err == nil || out.HTTPStatus != 503 {
		t.Fatalf("empty url should 503, got %+v %v", out, err)
	}
}
