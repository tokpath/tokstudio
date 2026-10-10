package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type flushCounter struct {
	http.ResponseWriter
	flushes int
}

func (f *flushCounter) Flush() {
	f.flushes++
	if flusher, ok := f.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func TestSSEWriterFlushesEveryChunk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	counter := &flushCounter{ResponseWriter: recorder}
	context, _ := gin.CreateTestContext(counter)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	writer := &sseWriter{c: context}
	if err := writer.Emit(`{"choices":[{"delta":{"content":"He"}}]}`); err != nil {
		t.Fatal(err)
	}
	if err := writer.Emit(`{"choices":[{"delta":{"content":"llo"}}]}`); err != nil {
		t.Fatal(err)
	}
	if counter.flushes != 2 {
		t.Fatalf("flushes=%d, each token must leave the buffer immediately", counter.flushes)
	}
	if !writer.started {
		t.Fatal("writer did not record that bytes were committed")
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "data: {\"choices\":[{\"delta\":{\"content\":\"He\"}}]}") || !strings.Contains(body, "llo") {
		t.Fatalf("body=%s", body)
	}
	if recorder.Header().Get("Content-Type") != "text/event-stream; charset=utf-8" {
		t.Fatalf("content-type=%s", recorder.Header().Get("Content-Type"))
	}
	if recorder.Header().Get("X-Accel-Buffering") != "no" {
		t.Fatal("proxy buffering header missing")
	}
}
