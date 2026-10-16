package remediation

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type RefundFeeInput struct {
	OperatorID, LegID, PaymentBusinessKey, BusinessKey, ActorID, Reference, Evidence string
	FeeUSD                                                                           ledger.Decimal
	OccurredAt                                                                       time.Time
}
type RefundFeeConfirmation struct {
	ID            string         `json:"id"`
	OperatorID    string         `json:"operator_id"`
	ObservationID string         `json:"observation_id"`
	LegID         string         `json:"leg_id"`
	BusinessKey   string         `json:"business_key"`
	Fingerprint   string         `json:"-"`
	FeeUSD        ledger.Decimal `json:"fee_usd"`
	JournalID     *string        `json:"journal_id,omitempty"`
	Reference     string         `json:"reference"`
	Evidence      string         `json:"evidence"`
	ActorID       string         `json:"actor_id"`
	OccurredAt    time.Time      `json:"occurred_at"`
	CreatedAt     time.Time      `json:"created_at"`
}

func (RefundFeeConfirmation) TableName() string { return "remediation_refund_fee_confirmations" }

type RefundPaymentAccounting struct {
	ID                string                     `json:"id"`
	LegID             string                     `json:"leg_id"`
	BusinessKey       string                     `json:"business_key"`
	ActualUSD         ledger.Decimal             `json:"actual_usd"`
	FeeStatus         string                     `json:"fee_status"`
	FeeUSD            *ledger.Decimal            `json:"fee_usd"`
	FeeConfirmationID *string                    `json:"fee_confirmation_id,omitempty"`
	Evidence          string                     `json:"evidence"`
	OccurredAt        time.Time                  `json:"occurred_at"`
	CreatedAt         time.Time                  `json:"created_at"`
	ExternalRefund    *RefundExternalObservation `json:"external_refund,omitempty" gorm:"-"`
}

type RefundExternalObservation struct {
	ObservationID      string `json:"observation_id"`
	AuthorizationID    string `json:"authorization_id"`
	RefundDestination  `gorm:"embedded"`
	TransactionHash    string    `json:"transaction_hash"`
	LogIndex           string    `json:"log_index"`
	BlockHash          string    `json:"block_hash"`
	SenderAddress      string    `json:"sender_address"`
	RecipientAddress   string    `json:"recipient_address"`
	Confirmations      string    `json:"confirmations"`
	VerificationMethod string    `json:"verification_method"`
	VerifiedBy         string    `json:"verified_by"`
	CreatedAt          time.Time `json:"created_at"`
}

func (RefundExternalObservation) TableName() string {
	return "remediation_external_refund_observations"
}

// ConfirmRefundFee adds the merchant's independently verified final fee. The
// original successful refund observation and its customer settlement stay intact.
func (s *Service) ConfirmRefundFee(ctx context.Context, tx *gorm.DB, in RefundFeeInput) (RefundFeeConfirmation, error) {
	var out RefundFeeConfirmation
	if !validID(in.OperatorID) || !validID(in.LegID) || in.PaymentBusinessKey == "" || in.BusinessKey == "" || len(in.BusinessKey) > 180 || in.Reference == "" || in.Evidence == "" || !in.FeeUSD.Valid() || in.FeeUSD.Sign() < 0 || in.OccurredAt.IsZero() {
		return out, ErrInvalid
	}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, in.ActorID, in.OperatorID, "refunds.manage"); e != nil {
			return e
		}
		_, leg, e := s.lockLeg(ctx, t, in.OperatorID, in.LegID)
		if e != nil {
			return e
		}
		var observation struct{ ID string }
		if e = t.Table("remediation_payment_observations").Select("id").Where("leg_id=? AND business_key=?", leg.ID, in.PaymentBusinessKey).Take(&observation).Error; e != nil {
			return e
		}
		out, e = s.recordRefundFee(ctx, t, leg, observation.ID, in)
		return e
	})
	return out, e
}
func (s *Service) recordRefundFee(ctx context.Context, t *gorm.DB, leg RefundLeg, observationID string, in RefundFeeInput) (RefundFeeConfirmation, error) {
	var out RefundFeeConfirmation
	if e := t.First(&out, "observation_id=?", observationID).Error; e == nil {
		if out.Fingerprint != fingerprint(in) {
			return out, ErrConflict
		}
		return out, nil
	} else if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, e
	}
	at, e := instant(t)
	if e != nil {
		return out, e
	}
	if in.OccurredAt.After(at.Add(5 * time.Minute)) {
		return out, ErrInvalid
	}
	out = RefundFeeConfirmation{ID: uuid.NewString(), OperatorID: leg.OperatorID, ObservationID: observationID, LegID: leg.ID, BusinessKey: in.BusinessKey, Fingerprint: fingerprint(in), FeeUSD: in.FeeUSD, Reference: in.Reference, Evidence: in.Evidence, ActorID: in.ActorID, OccurredAt: in.OccurredAt, CreatedAt: at}
	if in.FeeUSD.Sign() > 0 {
		cash, e := s.account(ctx, t, leg.OperatorID, leg.OperatorID, "control:external_cash")
		if e != nil {
			return out, e
		}
		fees, e := s.account(ctx, t, leg.OperatorID, leg.OperatorID, "control:merchant_fees")
		if e != nil {
			return out, e
		}
		journal, e := s.Ledger.Post(ctx, t, ledger.Posting{OperatorID: leg.OperatorID, BusinessKey: "refund-fee:" + observationID, Kind: "refund_merchant_fee", SourceID: observationID, ActorID: in.ActorID, Reason: in.Reference + " | " + in.Evidence, Lines: []ledger.Line{{AccountID: cash.ID, Amount: in.FeeUSD}, {AccountID: fees.ID, Amount: in.FeeUSD.Neg()}}})
		if e != nil {
			return out, e
		}
		out.JournalID = &journal.ID
	}
	if e = t.Create(&out).Error; e != nil {
		return out, e
	}
	return out, s.event(t, leg.OperatorID, leg.ID, "refund.merchant_fee_confirmed", in.ActorID, in.Evidence, out)
}
func (s *Service) RefundAccounting(ctx context.Context, tx *gorm.DB, actor, operator, legID string) ([]RefundPaymentAccounting, error) {
	rows := []RefundPaymentAccounting{}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, operator, "ledger.read"); e != nil {
			return e
		}
		var leg RefundLeg
		if e := t.First(&leg, "id=? AND operator_id=?", legID, operator).Error; e != nil {
			return e
		}
		if e := t.Table("remediation_payment_observations p").Select("p.id,p.leg_id,p.business_key,p.actual_usd,CASE WHEN f.id IS NOT NULL THEN 'confirmed' ELSE p.fee_status END AS fee_status,COALESCE(f.fee_usd,p.fee_usd) AS fee_usd,f.id AS fee_confirmation_id,p.evidence,p.occurred_at,p.created_at").Joins("LEFT JOIN remediation_refund_fee_confirmations f ON f.observation_id=p.id").Where("p.leg_id=?", legID).Order("p.created_at,p.id").Scan(&rows).Error; e != nil {
			return e
		}
		for i := range rows {
			var x RefundExternalObservation
			e := t.First(&x, "observation_id=?", rows[i].ID).Error
			if e == nil {
				rows[i].ExternalRefund = &x
			} else if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
		}
		return nil
	})
	return rows, e
}
