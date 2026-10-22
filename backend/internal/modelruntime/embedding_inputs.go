package modelruntime

import (
	"encoding/json"
	"github.com/maximhq/bifrost/core/schemas"
)

func embeddingInputs(raw json.RawMessage) ([]schemas.EmbeddingInputItem, error) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []schemas.EmbeddingInputItem{{Content: schemas.EmbeddingContent{{Type: schemas.EmbeddingContentPartTypeText, Text: &text}}}}, nil
	}
	var texts []string
	if json.Unmarshal(raw, &texts) == nil && len(texts) > 0 {
		out := make([]schemas.EmbeddingInputItem, len(texts))
		for i := range texts {
			out[i].Content = schemas.EmbeddingContent{{Type: schemas.EmbeddingContentPartTypeText, Text: &texts[i]}}
		}
		return out, nil
	}
	var tokens []int
	if json.Unmarshal(raw, &tokens) == nil && len(tokens) > 0 {
		return []schemas.EmbeddingInputItem{{Content: schemas.EmbeddingContent{{Type: schemas.EmbeddingContentPartTypeTokens, Tokens: tokens}}}}, nil
	}
	var groups [][]int
	if json.Unmarshal(raw, &groups) == nil && len(groups) > 0 {
		out := make([]schemas.EmbeddingInputItem, len(groups))
		for i := range groups {
			out[i].Content = schemas.EmbeddingContent{{Type: schemas.EmbeddingContentPartTypeTokens, Tokens: groups[i]}}
		}
		return out, nil
	}
	var out []schemas.EmbeddingInputItem
	if json.Unmarshal(raw, &out) != nil || len(out) == 0 {
		return nil, ErrInvalidRequest
	}
	return out, nil
}
