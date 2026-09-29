package gateway

import (
	"encoding/json"
	"strings"
)

// ParamError 表示请求带了协议明确不支持的参数。
type ParamError struct {
	Param string
}

func (e ParamError) Error() string {
	if e.Param == "" {
		return "unsupported parameter"
	}
	return "unsupported parameter " + e.Param
}

func (e ParamError) Unwrap() error { return ErrUnsupportedParam }

type ContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL json.RawMessage `json:"image_url,omitempty"`
	Source   json.RawMessage `json:"source,omitempty"`
}

type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

func (m *ChatMessage) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["role"]; ok {
		_ = json.Unmarshal(v, &m.Role)
	}
	if v, ok := raw["name"]; ok {
		_ = json.Unmarshal(v, &m.Name)
	}
	if v, ok := raw["tool_call_id"]; ok {
		_ = json.Unmarshal(v, &m.ToolCallID)
	}
	if v, ok := raw["tool_calls"]; ok {
		_ = json.Unmarshal(v, &m.ToolCalls)
	}
	if v, ok := raw["content"]; ok && len(v) > 0 && string(v) != "null" {
		var text string
		if err := json.Unmarshal(v, &text); err == nil {
			m.Content = text
			return nil
		}
		var parts []ContentPart
		if err := json.Unmarshal(v, &parts); err == nil {
			m.Parts = parts
			m.Content = flattenParts(parts)
		}
	}
	return nil
}

func (m ChatMessage) MarshalJSON() ([]byte, error) {
	type out struct {
		Role       string     `json:"role"`
		Content    any        `json:"content,omitempty"`
		Name       string     `json:"name,omitempty"`
		ToolCallID string     `json:"tool_call_id,omitempty"`
		ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	}
	row := out{Role: m.Role, Name: m.Name, ToolCallID: m.ToolCallID, ToolCalls: m.ToolCalls}
	if len(m.Parts) > 0 {
		row.Content = m.Parts
	} else if m.Content != "" {
		row.Content = m.Content
	}
	return json.Marshal(row)
}

func flattenParts(parts []ContentPart) string {
	var b strings.Builder
	for _, part := range parts {
		if part.Text != "" {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

func visionCount(req ChatRequest) int {
	n := 0
	for _, msg := range req.Messages {
		for _, part := range msg.Parts {
			if part.Type == "image_url" || part.Type == "image" {
				n++
			}
		}
	}
	return n
}

func jsonMode(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return false
	}
	kind, _ := body["type"].(string)
	return kind == "json_schema" || kind == "json_object"
}

func firstToolName(raw json.RawMessage) string {
	var tools []struct {
		Name     string `json:"name"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &tools); err == nil {
		for _, tool := range tools {
			if tool.Function.Name != "" {
				return tool.Function.Name
			}
			if tool.Name != "" {
				return tool.Name
			}
		}
	}
	return "echo"
}

func presentRaw(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s != "" && s != "null" && s != "{}" && s != "[]"
}

// ValidateChat 只拒绝协议层明确不支持的参数；模型能力不作为选路前门禁。
func ValidateChat(req ChatRequest) error {
	if presentRaw(req.LogitBias) {
		return ParamError{Param: "logit_bias"}
	}
	return nil
}
