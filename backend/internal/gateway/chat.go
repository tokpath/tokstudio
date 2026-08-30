package gateway

import (
	"encoding/json"
	"strings"
)

// ParamError 表示请求带了模型或协议明确不支持的参数。
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

func asStringSlice(v any) []string {
	switch items := v.(type) {
	case []string:
		return items
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			out = append(out, strings.TrimSpace(fmtString(item)))
		}
		return out
	default:
		return nil
	}
}

func fmtString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return strings.Trim(string(b), `"`)
}

func containsFold(list []string, want string) bool {
	for _, item := range list {
		if strings.EqualFold(strings.TrimSpace(item), want) {
			return true
		}
	}
	return false
}

func supportsParam(list []string, name string) bool {
	aliases := map[string][]string{
		"tools":     {"tools", "tool_choice"},
		"json":      {"json", "json_schema", "response_format"},
		"vision":    {"vision", "image"},
		"reasoning": {"reasoning", "reasoning_effort"},
	}
	for _, alias := range aliases[name] {
		if containsFold(list, alias) {
			return true
		}
	}
	return containsFold(list, name)
}

func presentRaw(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s != "" && s != "null" && s != "{}" && s != "[]"
}

// ValidateChat 按模型目录的支持/不支持参数做明确处理：能透传的留下，不能的结构化 4xx。
func ValidateChat(req ChatRequest, caps map[string]any) error {
	if presentRaw(req.LogitBias) {
		return ParamError{Param: "logit_bias"}
	}
	for _, name := range asStringSlice(caps["unsupported_parameters"]) {
		if name == "logit_bias" && presentRaw(req.LogitBias) {
			return ParamError{Param: name}
		}
	}
	supported := asStringSlice(caps["supported_parameters"])
	if len(supported) == 0 {
		return nil
	}
	checks := []struct {
		need bool
		name string
	}{
		{presentRaw(req.Tools) || presentRaw(req.ToolChoice), "tools"},
		{jsonMode(req.ResponseFormat), "json"},
		{visionCount(req) > 0, "vision"},
		{req.ReasoningEffort != "" || presentRaw(req.Reasoning), "reasoning"},
	}
	for _, check := range checks {
		if check.need && !supportsParam(supported, check.name) {
			return ParamError{Param: check.name}
		}
	}
	return nil
}
