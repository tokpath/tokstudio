// Package modelruntime executes one explicitly selected upstream attempt. It
// never authenticates customers, retries, selects a different model, or writes
// customer balances. Those decisions belong to the request coordinator.
package modelruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

type Operation string

const (
	Chat          Operation = "chat"
	Responses     Operation = "responses"
	Embedding     Operation = "embedding"
	Image         Operation = "image_generation"
	Speech        Operation = "speech"
	Transcription Operation = "transcription"
	Video         Operation = "video_generation"
	VideoRetrieve Operation = "video_retrieve"
	VideoDownload Operation = "video_download"
)

var (
	ErrUnsupported    = errors.New("unverified or unsupported capability")
	ErrUsageUnknown   = errors.New("reliable usage unavailable")
	ErrBoundExceeded  = errors.New("verified usage exceeds admitted bound")
	ErrInvalidRequest = errors.New("invalid model request")
)

// Capability is a probe result for one upstream model/protocol/mode combination.
// VerifiedAt alone is insufficient: evidence must cover parameter preservation,
// usage, bound enforcement and failure recovery before it can be advertised.
type Capability struct {
	Operation            Operation `json:"operation"`
	Streaming            bool      `json:"streaming"`
	InputTypes           []string  `json:"input_types"`
	Parameters           []string  `json:"parameters"`
	RequiredParameters   []string  `json:"required_parameters,omitempty"`
	MaxInputBytes        int       `json:"max_input_bytes"`
	MaxOutput            int64     `json:"max_output"`
	OutputLimitParameter string    `json:"output_limit_parameter,omitempty"`
	// BillableBounds has one finite quantity for every catalog price component.
	// The probe must prove these bounds for every allowed parameter combination.
	BillableBounds       map[string]string              `json:"billable_bounds"`
	ParameterConstraints map[string]ParameterConstraint `json:"parameter_constraints,omitempty"`
	UsageRules           []UsageRule                    `json:"usage_rules"`
	// SubtractPaths removes included sub-units before charging a separate
	// component (cached input or reasoning already included in output totals).
	SubtractPaths      map[string][]string `json:"subtract_paths,omitempty"`
	EvidenceID         string              `json:"evidence_id"`
	VerifiedAt         time.Time           `json:"verified_at"`
	ParametersVerified bool                `json:"parameters_verified"`
	UsageVerified      bool                `json:"usage_verified"`
	BoundsVerified     bool                `json:"bounds_verified"`
	RecoveryVerified   bool                `json:"recovery_verified"`
}
type ParameterConstraint struct {
	Min    string   `json:"min,omitempty"`
	Max    string   `json:"max,omitempty"`
	Values []string `json:"values,omitempty"`
}

// Path is an RFC 6901 pointer into the normalized provider usage. No monetary
// cost from the SDK is used: supplier costs are priced from exact unit facts.
type UsageRule struct {
	Component      string `json:"component"`
	Unit           string `json:"unit"`
	Path           string `json:"path"`
	ZeroWhenAbsent bool   `json:"zero_when_absent,omitempty"`
}

func (c Capability) Verified() bool {
	if !(c.EvidenceID != "" && !c.VerifiedAt.IsZero() && c.ParametersVerified && c.UsageVerified && c.BoundsVerified && c.RecoveryVerified && c.MaxInputBytes > 0 && len(c.BillableBounds) > 0 && len(c.UsageRules) > 0) {
		return false
	}
	for _, constraint := range c.ParameterConstraints {
		if len(constraint.Values) > 0 && (constraint.Min != "" || constraint.Max != "") {
			return false
		}
		var min, max *big.Rat
		var ok bool
		if constraint.Min != "" {
			min, ok = new(big.Rat).SetString(constraint.Min)
			if !ok {
				return false
			}
		}
		if constraint.Max != "" {
			max, ok = new(big.Rat).SetString(constraint.Max)
			if !ok {
				return false
			}
		}
		if min != nil && max != nil && min.Cmp(max) > 0 {
			return false
		}
	}
	seen := map[string]bool{}
	for _, r := range c.UsageRules {
		if r.Component == "" || r.Unit == "" || r.Path == "" || seen[r.Component] {
			return false
		}
		seen[r.Component] = true
		b, ok := new(big.Rat).SetString(c.BillableBounds[r.Component])
		if !ok || b.Sign() < 0 {
			return false
		}
	}
	if len(seen) != len(c.BillableBounds) {
		return false
	}
	return true
}

type Request struct {
	ID          string    `json:"request_id"`
	PublicModel string    `json:"model"`
	Operation   Operation `json:"operation"`
	Stream      bool      `json:"stream"`
	// Input and Params use Bifrost's canonical protocol schema. DecodeProtocol
	// below accepts standard endpoint bodies without losing unsupported fields.
	Input      json.RawMessage `json:"input"`
	Params     json.RawMessage `json:"params"`
	InputTypes []string        `json:"input_types"`
}

// Attempt contains references only. Secret resolution is platform owned and is
// performed immediately before execution, never exposed through request JSON.
type Attempt struct {
	ID            string     `json:"attempt_id"`
	RequestID     string     `json:"request_id"`
	UpstreamID    string     `json:"upstream_id"`
	Provider      string     `json:"provider"`
	UpstreamModel string     `json:"upstream_model"`
	PublicModel   string     `json:"public_model"`
	CredentialID  string     `json:"credential_id"`
	BaseURL       string     `json:"-"`
	Deadline      time.Time  `json:"deadline"`
	Capability    Capability `json:"capability"`
	TaskID        string     `json:"task_id,omitempty"`
}
type Credential struct {
	ID     string
	Secret string
}
type CredentialSource interface {
	ResolveCredential(context.Context, string, string) (Credential, error)
}

type Measurement struct {
	Component string `json:"component"`
	Unit      string `json:"unit"`
	Quantity  string `json:"quantity"`
}
type Usage struct {
	Source       string          `json:"source"` // provider_usage | verified_output | unknown
	Evidence     json.RawMessage `json:"evidence,omitempty"`
	Measurements []Measurement   `json:"measurements,omitempty"`
	Reliable     bool            `json:"reliable"`
}
type Result struct {
	AttemptID         string          `json:"attempt_id"`
	State             string          `json:"state"` // completed | submitted | failed | outcome_unknown
	Output            json.RawMessage `json:"-"`
	Binary            []byte          `json:"-"`
	ContentType       string          `json:"content_type,omitempty"`
	TaskID            string          `json:"task_id,omitempty"`
	ProviderRequestID string          `json:"provider_request_id,omitempty"`
	Usage             Usage           `json:"usage"`
	OutputDelivered   bool            `json:"output_delivered"`
	ErrorCode         string          `json:"error_code,omitempty"`
}
type Event struct {
	Data  json.RawMessage
	Usage *Usage
}

// Emit returns only after the downstream consumes the event. A non-nil error
// cancels the current upstream; it never triggers another generation.
type Emit func(context.Context, Event) error
type Adapter interface {
	Execute(context.Context, Attempt, Request, Emit) (Result, error)
}

func ValidateRequest(a Attempt, r Request) error {
	if a.ID == "" || a.RequestID != r.ID || r.ID == "" || r.PublicModel == "" || a.PublicModel != r.PublicModel || a.UpstreamModel == "" || a.CredentialID == "" || a.UpstreamID == "" || a.Deadline.IsZero() {
		return ErrInvalidRequest
	}
	c := a.Capability
	if r.Operation == VideoRetrieve || r.Operation == VideoDownload {
		if !c.Verified() || c.Operation != Video || r.Stream || a.TaskID == "" {
			return ErrUnsupported
		}
		return nil
	}
	if !c.Verified() || c.Operation != r.Operation || c.Streaming != r.Stream {
		return ErrUnsupported
	}
	if len(r.Input) == 0 || len(r.Input)+len(r.Params) > c.MaxInputBytes || !json.Valid(r.Input) {
		return ErrInvalidRequest
	}
	if r.Operation == Chat {
		var messages []struct {
			Role      string          `json:"role"`
			Content   json.RawMessage `json:"content"`
			ToolCalls json.RawMessage `json:"tool_calls"`
		}
		if json.Unmarshal(r.Input, &messages) != nil || len(messages) == 0 {
			return ErrInvalidRequest
		}
		for _, m := range messages {
			if !member([]string{"system", "developer", "user", "assistant", "tool"}, m.Role) {
				return ErrInvalidRequest
			}
			if len(m.Content) == 0 && len(m.ToolCalls) == 0 {
				return ErrInvalidRequest
			}
		}
	}
	if len(r.InputTypes) == 0 {
		return ErrInvalidRequest
	}
	var params map[string]json.RawMessage
	if len(r.Params) > 0 && json.Unmarshal(r.Params, &params) != nil {
		return ErrInvalidRequest
	}
	for k, v := range params {
		if string(v) == "null" {
			return fmt.Errorf("%w: null parameter %s", ErrInvalidRequest, k)
		}
		// Provider routing, caching and automatic tool loops never come from
		// customer parameters, even if a bad capability record allows them.
		switch k {
		case "store", "background":
			if string(v) != "false" {
				return fmt.Errorf("%w: managed parameter %s", ErrUnsupported, k)
			}
		case "fallbacks", "provider", "model", "api_key", "headers", "base_url", "extra_params", "previous_response_id":
			return fmt.Errorf("%w: managed parameter %s", ErrUnsupported, k)
		}
	}
	return nil
}
func member(values []string, key string) bool {
	for _, v := range values {
		if v == key {
			return true
		}
	}
	return false
}

// DecodeProtocol separates endpoint fields and parameters without decoding into
// a lossy subset first. Unknown parameters survive and are rejected by the
// verified capability/schema checks instead of silently disappearing.
func DecodeProtocol(id string, op Operation, body []byte) (Request, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || fields == nil {
		return Request{}, ErrInvalidRequest
	}
	r := Request{ID: id, Operation: op, InputTypes: []string{"text"}}
	if json.Unmarshal(fields["model"], &r.PublicModel) != nil || strings.TrimSpace(r.PublicModel) == "" {
		return Request{}, ErrInvalidRequest
	}
	delete(fields, "model")
	if raw, ok := fields["stream"]; ok {
		if json.Unmarshal(raw, &r.Stream) != nil {
			return Request{}, ErrInvalidRequest
		}
		delete(fields, "stream")
	}
	switch op {
	case Chat:
		r.Input = fields["messages"]
		delete(fields, "messages")
	case Responses:
		r.Input = fields["input"]
		delete(fields, "input")
		var text string
		if json.Unmarshal(r.Input, &text) == nil {
			r.Input, _ = json.Marshal([]map[string]any{{"role": "user", "content": []map[string]string{{"type": "input_text", "text": text}}}})
		}
	case Embedding:
		r.Input = fields["input"]
		delete(fields, "input")
	case Image, Video:
		in := map[string]json.RawMessage{}
		for _, k := range []string{"prompt", "input_reference", "video_uri"} {
			if raw, ok := fields[k]; ok {
				in[k] = raw
				delete(fields, k)
			}
		}
		r.Input, _ = json.Marshal(in)
	case Speech:
		r.Input, _ = json.Marshal(map[string]json.RawMessage{"input": fields["input"]})
		delete(fields, "input")
	case Transcription:
		in := map[string]json.RawMessage{}
		for _, k := range []string{"file", "filename"} {
			if raw, ok := fields[k]; ok {
				in[k] = raw
				delete(fields, k)
			}
		}
		r.Input, _ = json.Marshal(in)
		r.InputTypes = []string{"audio"}
	default:
		return Request{}, ErrUnsupported
	}
	if op == Chat {
		if raw, ok := fields["max_tokens"]; ok {
			if _, duplicate := fields["max_completion_tokens"]; duplicate {
				return Request{}, ErrInvalidRequest
			}
			fields["max_completion_tokens"] = raw
			delete(fields, "max_tokens")
		}
	}
	if op == Chat || op == Responses {
		r.InputTypes = detectInputTypes(r.Input)
	}
	r.Params, _ = json.Marshal(fields)
	return r, nil
}
func detectInputTypes(input json.RawMessage) []string {
	types := []string{"text"}
	var root any
	if json.Unmarshal(input, &root) != nil {
		return types
	}
	var visit func(any)
	visit = func(v any) {
		switch x := v.(type) {
		case []any:
			for _, item := range x {
				visit(item)
			}
		case map[string]any:
			if t, ok := x["type"].(string); ok {
				kind := ""
				switch t {
				case "image_url", "input_image":
					kind = "image"
				case "input_audio", "audio":
					kind = "audio"
				case "video_url", "input_video":
					kind = "video"
				}
				if kind != "" && !member(types, kind) {
					types = append(types, kind)
				}
			}
			for _, item := range x {
				visit(item)
			}
		}
	}
	visit(root)
	return types
}
