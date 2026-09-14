package gateway

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBailianLiveHitsDashScopeWhenKeyPresent(t *testing.T) {
	key := strings.TrimSpace(os.Getenv("TOKENHUB_BAILIAN_API_KEY"))
	if key == "" {
		t.Skip("live Bailian requires TOKENHUB_BAILIAN_API_KEY")
	}
	base := strings.TrimSpace(os.Getenv("TOKENHUB_BAILIAN_BASE_URL"))
	if base == "" {
		base = "https://coding.dashscope.aliyuncs.com/v1"
	}
	model := strings.TrimSpace(os.Getenv("TOKENHUB_BAILIAN_MODEL"))
	if model == "" {
		model = "qwen3.7-plus"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	rt, err := Start(ctx, Settings{OpenAIAPIKey: key, LogLevel: "error"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)

	callCtx := context.WithValue(ctx, ctxProviderBaseURLKey, base)
	callCtx = context.WithValue(callCtx, ctxAccountSecretKey, key)
	out, err := BifrostAdapter{Runtime: rt}.Chat(callCtx, "bailian", "", ChatRequest{
		Model:    model,
		Messages: []ChatMessage{{Role: "user", Content: "Reply with the single word pong."}},
	})
	if err != nil || out.HTTPStatus != 200 {
		t.Fatalf("live bailian failed: status=%d err=%v body=%+v", out.HTTPStatus, err, out.Body)
	}
	if len(out.Body.Choices) == 0 || strings.TrimSpace(out.Body.Choices[0].Message.Content) == "" {
		t.Fatalf("live bailian empty content: %+v", out.Body)
	}
	if strings.Contains(out.Body.Choices[0].Message.Content, "echo:") {
		t.Fatalf("live bailian must not echo: %+v", out.Body)
	}
}
