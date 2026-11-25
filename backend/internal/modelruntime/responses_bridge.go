package modelruntime

import (
	"context"
	"encoding/json"

	"github.com/maximhq/bifrost/core/providers/anthropic"
	"github.com/maximhq/bifrost/core/schemas"
)

// ResponsesFromClient adapts a Chat Completions or Messages request to an
// existing subscription Responses capability.
func ResponsesFromClient(r Request) (Request, error) {
	switch r.Operation {
	case Chat:
		return responsesFromChat(r)
	case Messages:
		return responsesFromMessages(r)
	default:
		return Request{}, ErrUnsupported
	}
}

func responsesFromChat(r Request) (Request, error) {
	var chat schemas.BifrostChatRequest
	if err := decodePreserved(r.Input, &chat.Input); err != nil {
		return Request{}, err
	}
	if len(chat.Input) == 0 {
		return Request{}, ErrInvalidRequest
	}
	for _, msg := range chat.Input {
		if !member([]string{"system", "developer", "user", "assistant", "tool"}, string(msg.Role)) {
			return Request{}, ErrInvalidRequest
		}
	}
	chat.Model = r.PublicModel
	chat.Params = &schemas.ChatParameters{}
	if err := decodePreserved(r.Params, chat.Params); err != nil {
		return Request{}, err
	}
	if err := bridgeParameters(r.Params, []string{
		"max_completion_tokens", "stream_options", "n", "tools", "tool_choice", "parallel_tool_calls",
		"response_format", "reasoning_effort", "reasoning", "verbosity", "temperature", "top_p", "service_tier",
	}); err != nil {
		return Request{}, err
	}
	if chat.Params.N != nil && *chat.Params.N != 1 {
		return Request{}, ErrUnsupported
	}
	var options struct {
		StreamOptions struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options"`
	}
	_ = json.Unmarshal(r.Params, &options)
	r.ClientProtocol, r.IncludeUsage = Chat, options.StreamOptions.IncludeUsage
	converted := chat.ToResponsesRequest()
	if converted.Params.StreamOptions != nil && converted.Params.StreamOptions.IncludeObfuscation == nil {
		converted.Params.StreamOptions = nil
	}
	return bridgeResponsesInput(r, converted)
}

func responsesFromMessages(r Request) (Request, error) {
	if len(r.Input) == 0 {
		return Request{}, ErrInvalidRequest
	}
	if err := bridgeParameters(r.Params, []string{
		"system", "max_tokens", "tools", "tool_choice",
		"thinking", "output_config", "output_format", "temperature", "top_p",
	}); err != nil {
		return Request{}, err
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(r.Params, &body); err != nil || body == nil {
		return Request{}, ErrInvalidRequest
	}
	body["messages"] = r.Input
	body["model"], _ = json.Marshal(r.PublicModel)
	body["stream"], _ = json.Marshal(r.Stream)
	raw, err := json.Marshal(body)
	if err != nil {
		return Request{}, ErrInvalidRequest
	}
	var messages anthropic.AnthropicMessageRequest
	if err := decodePreserved(raw, &messages); err != nil {
		return Request{}, err
	}
	if messages.MaxTokens < 1 || len(messages.Messages) == 0 {
		return Request{}, ErrInvalidRequest
	}
	for _, msg := range messages.Messages {
		if !member([]string{"user", "assistant"}, string(msg.Role)) {
			return Request{}, ErrUnsupported
		}
	}
	r.ClientProtocol = Messages
	bc := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	return bridgeResponsesInput(r, messages.ToBifrostResponsesRequest(bc))
}

func bridgeParameters(raw json.RawMessage, allowed []string) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return ErrInvalidRequest
	}
	for key, value := range fields {
		if !member(allowed, key) || string(value) == "null" {
			return ErrUnsupported
		}
	}
	return nil
}

func bridgeResponsesInput(r Request, converted *schemas.BifrostResponsesRequest) (Request, error) {
	if converted == nil || len(converted.Input) == 0 {
		return Request{}, ErrInvalidRequest
	}
	var err error
	r.Operation = Responses
	r.Input, err = json.Marshal(converted.Input)
	if err != nil {
		return Request{}, ErrInvalidRequest
	}
	r.Params, err = json.Marshal(converted.Params)
	if err != nil {
		return Request{}, ErrInvalidRequest
	}
	r.InputTypes = detectInputTypes(r.Input)
	return r, nil
}

type responseStreamClient struct {
	request Request
	ctx     *schemas.BifrostContext
	id      string
	created int
}

func (c *responseStreamClient) events(chunk *schemas.BifrostStreamChunk) ([]Event, error) {
	if c.request.ClientProtocol == "" {
		body, err := publicOutput(chunk, c.request.PublicModel)
		return []Event{{Data: body}}, err
	}
	response := chunk.BifrostResponsesStreamResponse
	if response == nil {
		return nil, ErrInvalidRequest
	}
	if response.Type == schemas.ResponsesStreamResponseTypeFailed || response.Type == schemas.ResponsesStreamResponseTypeError ||
		failedResponsesResponse(response.Response) {
		return nil, &UpstreamFailure{}
	}
	switch c.request.ClientProtocol {
	case Chat:
		reply := response.ToBifrostChatResponse()
		if reply == nil {
			return nil, nil
		}
		if response.Response != nil && response.Response.ID != nil {
			c.id = *response.Response.ID
		}
		if reply.Created != 0 {
			c.created = reply.Created
		}
		reply.ID, reply.Created = c.id, c.created
		usage := reply.Usage
		reply.Usage = nil
		body, err := publicOutput(reply, c.request.PublicModel)
		events := []Event{{Data: body}}
		if err == nil && c.request.IncludeUsage && usage != nil {
			reply.Choices = []schemas.BifrostResponseChoice{}
			reply.Usage = usage
			body, err = publicOutput(reply, c.request.PublicModel)
			events = append(events, Event{Data: body})
		}
		return events, err
	case Messages:
		if response.Response != nil {
			safe := *response.Response
			safe.Model = c.request.PublicModel
			event := *response
			event.Response = &safe
			response = &event
		}
		var events []Event
		for _, item := range anthropic.ToAnthropicResponsesStreamResponse(c.ctx, response) {
			body, err := json.Marshal(item)
			if err != nil {
				return nil, err
			}
			events = append(events, Event{Data: body, Type: string(item.Type)})
		}
		return events, nil
	default:
		return nil, ErrUnsupported
	}
}

func clientModelOutput(ctx context.Context, response any, r Request) (json.RawMessage, error) {
	if r.ClientProtocol == "" {
		return publicOutput(response, r.PublicModel)
	}
	responses, ok := response.(*schemas.BifrostResponsesResponse)
	if !ok || responses == nil {
		return nil, ErrInvalidRequest
	}
	if failedResponsesResponse(responses) {
		return nil, &UpstreamFailure{}
	}
	switch r.ClientProtocol {
	case Chat:
		return publicOutput(responses.ToBifrostChatResponse(), r.PublicModel)
	case Messages:
		safe := *responses
		safe.Model = r.PublicModel
		return publicOutput(anthropic.ToAnthropicResponsesResponse(schemas.NewBifrostContext(ctx, schemas.NoDeadline), &safe), r.PublicModel)
	default:
		return nil, ErrUnsupported
	}
}

func failedResponsesResponse(response *schemas.BifrostResponsesResponse) bool {
	return response != nil && (response.Error != nil || response.Status != nil && *response.Status == schemas.ResponsesResponseStatusFailed)
}
