package app

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tokpath/tokstudio/backend/internal/gateway"
)

// sseWriter 把每一个 chat chunk 立刻写成 SSE 并 Flush。
// 第一个字节写出之后 started 为 true，调用方不能再改成 JSON 错误体。
type sseWriter struct {
	c       *gin.Context
	started bool
}

func (w *sseWriter) Emit(chunk string) error {
	if w == nil || w.c == nil {
		return nil
	}
	if !w.started {
		w.c.Header("Content-Type", "text/event-stream; charset=utf-8")
		w.c.Header("Cache-Control", "no-cache, no-transform")
		w.c.Header("Connection", "keep-alive")
		w.c.Header("X-Accel-Buffering", "no")
		w.c.Status(http.StatusOK)
	}
	if _, err := w.c.Writer.WriteString("data: " + chunk + "\n\n"); err != nil {
		w.started = true
		return err
	}
	w.c.Writer.Flush()
	w.started = true
	return nil
}

var _ gateway.StreamSink = (*sseWriter)(nil)
