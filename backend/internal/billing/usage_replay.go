package billing

import (
	"context"
	"errors"
)

// ReplayActualUsage keeps the original request, customer, model and frozen
// prices. Management cannot turn a reconciliation into a different request.
func (s *Service) ReplayActualUsage(ctx context.Context, requestID string, usage map[string]int) (*Settlement, error) {
	for key, value := range usage {
		if value < 0 || (key != "prompt_tokens" && key != "completion_tokens" && key != "reasoning_tokens") {
			return nil, ErrInvalidAmount
		}
	}
	if _, ok := usage["prompt_tokens"]; !ok {
		return nil, ErrInvalidAmount
	}
	if _, ok := usage["completion_tokens"]; !ok {
		return nil, ErrInvalidAmount
	}
	gap, err := s.GetUsageGap(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if gap.State == UsageConfirmed {
		prompt, completion, reasoning := ParseUnitUsage(gap.UnitUsage)
		if prompt != int64(usage["prompt_tokens"]) || completion != int64(usage["completion_tokens"]) || reasoning != int64(usage["reasoning_tokens"]) {
			return nil, ErrConflict
		}
		charges, err := s.ListChargesByRequest(ctx, requestID)
		if err != nil {
			return nil, err
		}
		if len(charges) == 0 {
			return nil, ErrNotFound
		}
		return &charges[0], nil
	}
	if gap.State != UsagePending {
		return nil, ErrAuthNotReserved
	}
	in := SettleInput{RequestID: gap.RequestID, UserID: gap.UserID, APIKeyID: gap.APIKeyID, ChannelOrgID: gap.ChannelOrgID, PublicModelID: gap.PublicModelID, ProviderID: gap.ProviderID, UpstreamModelID: gap.UpstreamModelID, AttemptID: gap.AttemptID, FactSource: gap.FactSource, Usage: usage}
	result, err := s.Settle(ctx, in)
	if errors.Is(err, ErrAlreadyCharged) {
		return nil, ErrConflict
	}
	return result, err
}
