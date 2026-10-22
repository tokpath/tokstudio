package modelpricing

import (
	"context"
	"encoding/json"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"strconv"
)

func (s *Service) Estimate(ctx context.Context, snapshot json.RawMessage, r modelruntime.Request, c modelruntime.Capability) *string {
	quantities := modelruntime.EstimateUsage(c, r)
	generic := modelruntime.DefaultBilling(modelruntime.Chat).Capability(r.Operation, false)
	tokenEstimates := modelruntime.EstimateUsage(generic, r)
	for _, key := range []string{"input", "output"} {
		if quantities[key] == "" {
			quantities[key] = tokenEstimates[key]
		}
	}
	var params map[string]json.RawMessage
	_ = json.Unmarshal(r.Params, &params)
	number := func(key string) int {
		n, _ := strconv.ParseFloat(quantities[key], 64)
		if n < 0 || n > 1e9 {
			return 0
		}
		return int(n)
	}
	seconds := "60"
	if r.Operation == modelruntime.Transcription {
		if value := quantities["input_audio_seconds"]; value != "" {
			seconds = value
		}
	} else if value := quantities["output_video_seconds"]; value != "" {
		seconds = value
	}
	var n int
	_ = json.Unmarshal(params["n"], &n)
	if n < 1 {
		n = 1
	}
	if n > 10000 {
		return nil
	}
	usage := map[string]any{"prompt_tokens": number("input"), "completion_tokens": number("output"), "total_tokens": number("input") + number("output"), "input_tokens": number("input"), "output_tokens": number("output"), "input_chars": number("input_text_characters"), "seconds": json.Number(seconds)}
	body := map[string]any{"usage": usage, "status": "completed", "seconds": seconds, "extra_fields": map[string]any{"request_type": requestType(r.Operation), "raw_response": map[string]any{"status": "completed", "seconds": seconds}}}
	for _, key := range []string{"size", "quality"} {
		var value string
		_ = json.Unmarshal(params[key], &value)
		body[key] = value
	}
	images := make([]map[string]string, n)
	for i := range images {
		images[i] = map[string]string{"url": "https://estimate.invalid/image"}
	}
	body["data"] = images
	raw, _ := json.Marshal(body)
	var response any
	switch r.Operation {
	case modelruntime.Chat:
		response = &schemas.BifrostChatResponse{}
	case modelruntime.Responses:
		response = &schemas.BifrostResponsesResponse{}
	case modelruntime.Embedding:
		response = &schemas.BifrostEmbeddingResponse{}
	case modelruntime.Image:
		response = &schemas.BifrostImageGenerationResponse{}
	case modelruntime.Speech:
		response = &schemas.BifrostSpeechResponse{}
	case modelruntime.Transcription:
		response = &schemas.BifrostTranscriptionResponse{}
	case modelruntime.Video:
		response = &schemas.BifrostVideoGenerationResponse{}
	default:
		return nil
	}
	if json.Unmarshal(raw, response) != nil {
		return nil
	}
	result := s.Observe(ctx, modelruntime.Attempt{CostSnapshot: snapshot}, response)
	return result.CalculatedUSD
}

func requestType(op modelruntime.Operation) schemas.RequestType {
	switch op {
	case modelruntime.Chat:
		return schemas.ChatCompletionRequest
	case modelruntime.Responses:
		return schemas.ResponsesRequest
	case modelruntime.Embedding:
		return schemas.EmbeddingRequest
	case modelruntime.Image:
		return schemas.ImageGenerationRequest
	case modelruntime.Speech:
		return schemas.SpeechRequest
	case modelruntime.Transcription:
		return schemas.TranscriptionRequest
	case modelruntime.Video:
		return schemas.VideoGenerationRequest
	}
	return ""
}
