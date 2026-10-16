package modelruntime

import (
	"github.com/maximhq/bifrost/core/schemas"
	"testing"
)

func TestStreamZeroRequiresNativeProviderEvidence(t *testing.T) {
	c := testCapability(Chat, true)
	r := &schemas.BifrostStreamChunk{BifrostChatResponse: &schemas.BifrostChatResponse{Usage: &schemas.BifrostLLMUsage{}}}
	if extractUsage(r, c).Reliable {
		t.Fatal("SDK synthetic empty usage was accepted")
	}
	r.BifrostChatResponse.ExtraFields.RawResponse = `{"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`
	if u := extractUsage(r, c); !u.Reliable || len(u.Measurements) != 2 {
		t.Fatal("native zero must remain valid", u)
	}
	r.BifrostChatResponse.ExtraFields.RawResponse = `{"usage":{"prompt_tokens":0,"completion_tokens":0}}`
	if extractUsage(r, c).Reliable {
		t.Fatal("incomplete usage was accepted")
	}
}
