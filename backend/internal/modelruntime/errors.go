package modelruntime

import (
	"context"
	"errors"
)

// UpstreamFailure deliberately cannot hold supplier-controlled text. Keeping
// only a status makes Error(), logs and wrapped errors safe by construction.
type UpstreamFailure struct {
	Status int
}

func (e *UpstreamFailure) Error() string {
	if e == nil {
		return "upstream_error"
	}
	switch SafeProviderStatus(e.Status) {
	case 401, 403:
		return "upstream_authentication_failed"
	case 429:
		return "rate_limited"
	case 408, 504:
		return "upstream_timeout"
	default:
		return "upstream_error"
	}
}

func SafeProviderStatus(status int) int {
	if status < 100 || status > 599 {
		return 0
	}
	return status
}

// SafeErrorCode accepts system-authored constants, never SDK strings. Use this
// again at persistence boundaries, including implementations other than Bifrost.
func SafeErrorCode(code string) string {
	switch code {
	case "", "upstream_error", "upstream_authentication_failed", "rate_limited",
		"upstream_timeout", "request_cancelled", "request_not_dispatched",
		"input_quantity_unavailable", "route_contract_violation",
		"unsupported_capability", "invalid_model_request", "usage_unavailable",
		"usage_bound_exceeded", "model_execution_failed", "usage_unverified_before_deadline":
		return code
	default:
		return "model_execution_failed"
	}
}

// Diagnostic classifies errors without ever formatting their free text or
// their wrappers: transport errors may contain credential-bearing URLs.
func Diagnostic(err error) string {
	if err == nil {
		return ""
	}
	var upstream *UpstreamFailure
	if errors.As(err, &upstream) {
		return upstream.Error()
	}
	for _, item := range []struct {
		err  error
		code string
	}{
		{context.Canceled, "request_cancelled"},
		{context.DeadlineExceeded, "upstream_timeout"},
		{ErrUnsupported, "unsupported_capability"},
		{ErrInvalidRequest, "invalid_model_request"},
		{ErrUsageUnknown, "usage_unavailable"},
		{ErrBoundExceeded, "usage_bound_exceeded"},
	} {
		if errors.Is(err, item.err) {
			return item.code
		}
	}
	return "model_execution_failed"
}
