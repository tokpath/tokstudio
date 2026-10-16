package incentives

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

func Migrations() (string, fs.FS) {
	f, err := fs.Sub(migrationFS, "migrate")
	if err != nil {
		panic(err)
	}
	return "incentives", f
}

type Service struct {
	db             *gorm.DB
	org            *organization.Service
	ledger         *ledger.Service
	commerce       *commerce.Service
	rewardCoverage RegistrationCoverage
}

func New(db *gorm.DB) *Service {
	return &Service{db: db, org: organization.New(db), ledger: ledger.New(db), commerce: commerce.New(db)}
}
func (s *Service) SetRegistrationCoverage(c RegistrationCoverage) { s.rewardCoverage = c }
func (s *Service) within(ctx context.Context, tx *gorm.DB, f func(*gorm.DB) error) error {
	if tx != nil {
		return classify(f(tx.WithContext(ctx)))
	}
	return classify(s.db.WithContext(ctx).Transaction(f))
}
func (s *Service) dbFor(ctx context.Context, tx *gorm.DB) *gorm.DB {
	if tx == nil {
		tx = s.db
	}
	return tx.WithContext(ctx)
}
func classify(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) && p.Code == "23505" {
		return ErrConflict
	}
	return err
}
func validID(id string) bool { _, err := uuid.Parse(id); return err == nil }
func fingerprint(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// One operator lock serializes qualification clocks and incentive ledgers.
// This conservative boundary avoids eligibility races and reverse account
// locks; independent operating entities remain fully concurrent.
func lockOperator(tx *gorm.DB, operator string) error {
	return tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "incentives:"+operator).Error
}

// LockBusiness is the shared production fence: organization ownership must be
// fenced by the caller first, then cooperation, incentives and ledger. Usage
// and expiry observers must acquire this before mutating ledger source state.
func (s *Service) LockBusiness(ctx context.Context, tx *gorm.DB, operator string) error {
	if tx == nil || !validID(operator) {
		return ErrInvalidInput
	}
	if locker, ok := s.rewardCoverage.(interface {
		Lock(context.Context, *gorm.DB, string) error
	}); ok {
		if err := locker.Lock(ctx, tx, operator); err != nil {
			return err
		}
	}
	return lockOperator(tx.WithContext(ctx), operator)
}

type BusinessInstant struct {
	At       time.Time
	Sequence int64
}

func (s *Service) BusinessInstant(ctx context.Context, tx *gorm.DB) (BusinessInstant, error) {
	var out BusinessInstant
	err := s.dbFor(ctx, tx).Raw(`SELECT clock_timestamp() AS at,nextval('incentives_business_sequence') AS sequence`).Scan(&out).Error
	out.At = out.At.UTC()
	return out, err
}
func (s *Service) event(tx *gorm.DB, operator, aggregate, kind string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO incentives_events(id,operator_id,aggregate_id,kind,payload,created_at) VALUES(?,?,?,?,?::jsonb,clock_timestamp())`, uuid.NewString(), operator, aggregate, kind, string(b)).Error
}
func (s *Service) account(ctx context.Context, tx *gorm.DB, operator, customer, purpose string) (ledger.Account, error) {
	return s.ledger.EnsureAccount(ctx, tx, ledger.Account{OperatorID: operator, OwnerID: customer, Purpose: purpose, Unit: "USD", AllowNegative: len(purpose) >= 8 && purpose[:8] == "control:"})
}
func minimum(a, b ledger.Decimal) ledger.Decimal {
	if a.Cmp(b) < 0 {
		return a
	}
	return b
}
func rateValid(v ledger.Decimal) bool {
	return v.Valid() && v.Sign() >= 0 && v.Cmp(ledger.FromInt64(1)) <= 0
}

type policyRow Policy

func (policyRow) TableName() string { return "incentives_policies" }

type qualificationRow Qualification

func (qualificationRow) TableName() string { return "incentives_qualification_accounts" }

type functionRow FunctionState

func (functionRow) TableName() string { return "incentives_function_states" }

type campaignRow Campaign

func (campaignRow) TableName() string { return "incentives_campaigns" }

type campaignVersionRow CampaignVersion

func (campaignVersionRow) TableName() string { return "incentives_campaign_versions" }

type snapshotRow OrderSnapshot

func (snapshotRow) TableName() string { return "incentives_order_snapshots" }

type confirmationRow ConfirmationEvent

func (confirmationRow) TableName() string { return "incentives_confirmation_events" }

type commissionRow CommissionEntry

func (commissionRow) TableName() string { return "incentives_commission_entries" }

type withdrawalPolicyRow WithdrawalPolicy

func (withdrawalPolicyRow) TableName() string { return "incentives_withdrawal_policies" }

type withdrawalRow Withdrawal

func (withdrawalRow) TableName() string { return "incentives_withdrawals" }

type promiseRow RegistrationPromise

func (promiseRow) TableName() string { return "incentives_registration_promises" }
func (Event) TableName() string      { return "incentives_events" }

func (s *Service) SetPolicy(ctx context.Context, tx *gorm.DB, actorID, operatorID string, in PolicyInput) (out Policy, err error) {
	if !validID(operatorID) || in.ExpectedVersion < 0 || in.ThresholdUSD.Sign() < 0 || !in.ThresholdUSD.Valid() || !rateValid(in.DirectRate) || !rateValid(in.IndirectRate) || in.DirectRate.Add(in.IndirectRate).Cmp(ledger.FromInt64(1)) > 0 || in.ConfirmationSeconds < 0 || in.ConfirmationSeconds > 31536000 || in.InviteeATG.Sign() < 0 || in.InviterATG.Sign() < 0 || !in.InviteeATG.Valid() || !in.InviterATG.Valid() || in.RewardValiditySeconds <= 0 || in.RewardValiditySeconds > 315360000 || in.KOLCrossActivityLimit < 0 {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		if err := s.org.RequirePermission(ctx, tx, actorID, operatorID, "marketing.manage"); err != nil {
			return err
		}
		if err := s.LockBusiness(ctx, tx, operatorID); err != nil {
			return err
		}
		var current policyRow
		q := tx.Where("operator_id=?", operatorID).Order("version DESC").Limit(1).Find(&current)
		if q.Error != nil {
			return q.Error
		}
		if current.Version != in.ExpectedVersion {
			return ErrConflict
		}
		instant, err := s.BusinessInstant(ctx, tx)
		if err != nil {
			return err
		}
		r := policyRow{ID: uuid.NewString(), OperatorID: operatorID, Version: current.Version + 1, ThresholdUSD: in.ThresholdUSD, DirectRate: in.DirectRate, IndirectRate: in.IndirectRate, ConfirmationSeconds: in.ConfirmationSeconds, InviteeATG: in.InviteeATG, InviterATG: in.InviterATG, RewardValiditySeconds: in.RewardValiditySeconds, KOLCrossActivityLimit: in.KOLCrossActivityLimit, EffectiveAt: instant.At, BusinessSequence: instant.Sequence}
		if err = tx.Create(&r).Error; err != nil {
			return err
		}
		out = Policy(r)
		var before any
		if q.RowsAffected > 0 {
			before = Policy(current)
		}
		return s.event(tx, operatorID, r.ID, "policy.changed", map[string]any{"actor_id": actorID, "evidence": in.Evidence, "before": before, "after": out})
	})
	return
}
func (s *Service) policyAt(tx *gorm.DB, operator string, at time.Time) (Policy, error) {
	var r policyRow
	q := tx.Where("operator_id=? AND effective_at<=?", operator, at).Order("effective_at DESC,business_sequence DESC").Limit(1).Find(&r)
	if q.Error != nil {
		return Policy{}, q.Error
	}
	if q.RowsAffected == 0 {
		return Policy{}, ErrNotReady
	}
	return Policy(r), nil
}
func (s *Service) CurrentPolicy(ctx context.Context, tx *gorm.DB, operator string) (Policy, error) {
	instant, err := s.BusinessInstant(ctx, tx)
	if err != nil {
		return Policy{}, err
	}
	return s.policyAt(s.dbFor(ctx, tx), operator, instant.At)
}
func (s *Service) ensureQualification(tx *gorm.DB, operator, customer string) (Qualification, error) {
	r := qualificationRow{OperatorID: operator, CustomerID: customer}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "operator_id"}, {Name: "customer_id"}}, DoNothing: true}).Create(&r).Error; err != nil {
		return Qualification{}, err
	}
	if err := tx.First(&r, "operator_id=? AND customer_id=?", operator, customer).Error; err != nil {
		return Qualification{}, err
	}
	return Qualification(r), nil
}
func (s *Service) stateAt(tx *gorm.DB, operator, customer string, at time.Time) (FunctionState, error) {
	var r functionRow
	q := tx.Where("operator_id=? AND customer_id=? AND effective_at<=?", operator, customer, at).Order("effective_at DESC,business_sequence DESC").Limit(1).Find(&r)
	if q.Error != nil {
		return FunctionState{}, q.Error
	}
	if q.RowsAffected == 0 {
		return FunctionState{OperatorID: operator, CustomerID: customer, Status: "active"}, nil
	}
	return FunctionState(r), nil
}
func (s *Service) SetFunctionState(ctx context.Context, tx *gorm.DB, actor, operator, customer, status, evidence string, blocked bool) (out FunctionState, err error) {
	if !validID(customer) || (status != "active" && status != "suspended" && status != "terminated") || evidence == "" {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		if err := s.org.RequirePermission(ctx, tx, actor, operator, "marketing.manage"); err != nil {
			return err
		}
		if err := s.LockBusiness(ctx, tx, operator); err != nil {
			return err
		}
		own, err := s.org.CurrentOwnership(ctx, tx, customer)
		if err != nil {
			return err
		}
		if own.OperatorID != operator {
			return ErrNotEligible
		}
		instant, err := s.BusinessInstant(ctx, tx)
		if err != nil {
			return err
		}
		if err = s.advance(ctx, tx, operator, customer, instant.At); err != nil {
			return err
		}
		current, err := s.stateAt(tx, operator, customer, instant.At)
		if err != nil {
			return err
		}
		if current.Status == "terminated" && status != "terminated" {
			return ErrConflict
		}
		r := functionRow{ID: uuid.NewString(), OperatorID: operator, CustomerID: customer, Status: status, ReleaseBlocked: blocked, EvidenceReference: evidence, ActorID: actor, EffectiveAt: instant.At, BusinessSequence: instant.Sequence}
		if err = tx.Create(&r).Error; err != nil {
			return err
		}
		out = FunctionState(r)
		return s.event(tx, operator, customer, "commission.function_changed", out)
	})
	return
}
