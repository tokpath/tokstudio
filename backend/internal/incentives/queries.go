package incentives

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

// PreviewQuote uses a savepoint to exercise the same eligibility calculations
// as an order and then rolls back all trial counters and snapshots.
// The returned Reference is empty: a preview is never a reward promise.
func (s *Service) PreviewQuote(ctx context.Context, tx *gorm.DB, customer, operator, quoteID string, selection json.RawMessage) (out commerce.MarketingQuote, err error) {
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		own, err := s.org.CurrentOwnership(ctx, tx, customer)
		if err != nil {
			return err
		}
		if own.OperatorID != operator {
			return ErrNotEligible
		}
		quote, err := s.commerce.GetQuote(ctx, tx, quoteID)
		if err != nil {
			return err
		}
		if quote.OperatorID != operator {
			return ErrNotEligible
		}
		id := uuid.NewString()
		point := "incentives_preview_" + strings.ReplaceAll(id, "-", "")
		if err = tx.SavePoint(point).Error; err != nil {
			return err
		}
		o := commerce.Order{ID: id, CustomerID: customer, OperatorID: operator, BrandID: own.BrandID, OwnershipVersion: own.Version, QuoteID: &quote.ID, DueMoney: quote.PriceMoney, PaidATG: quote.PaidATG, PackageGiftATG: quote.GiftATG}
		out, err = s.Quote(ctx, tx, o, selection)
		rollback := tx.RollbackTo(point).Error
		if rollback != nil {
			return rollback
		}
		if err != nil {
			return err
		}
		out.Reference = ""
		return nil
	})
	return
}

type ActivityOffer struct {
	Name                  string          `json:"name"`
	ID                    string          `json:"id"`
	Kind                  string          `json:"kind"`
	OwnerCustomerID       *string         `json:"owner_customer_id"`
	PackageID             string          `json:"package_id"`
	FinalPriceMoney       *ledger.Decimal `json:"final_price_money"`
	GiftATG               ledger.Decimal  `json:"gift_atg"`
	ConcessionRate        ledger.Decimal  `json:"concession_rate"`
	StartsAt              time.Time       `json:"starts_at"`
	EndsAt                time.Time       `json:"ends_at"`
	PerCustomerLimit      int64           `json:"per_customer_limit"`
	Eligible              bool            `json:"eligible"`
	Reason                string          `json:"reason"`
	AllowCombination      bool            `json:"allow_combination"`
	CompatibleCampaignIDs []string        `json:"compatible_campaign_ids"`
}

func (s *Service) EligibleActivities(ctx context.Context, tx *gorm.DB, operator, customer, packageID string) (out []ActivityOffer, err error) {
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		own, err := s.org.CurrentOwnership(ctx, tx, customer)
		if err != nil {
			return err
		}
		if own.OperatorID != operator {
			return ErrNotEligible
		}
		p, err := s.commerce.GetPackage(ctx, tx, operator, packageID)
		if err != nil {
			return err
		}
		if p.CurrentQuoteID == nil {
			return ErrNotReady
		}
		chain, err := s.org.ReferralChain(ctx, tx, customer)
		if err != nil {
			return err
		}
		direct := ""
		if len(chain) > 0 {
			direct = chain[0].InviterID
		}
		var campaigns []campaignRow
		q := tx.Where("operator_id=?", operator)
		if direct == "" {
			q = q.Where("kind='merchant'")
		} else {
			q = q.Where("kind='merchant' OR owner_customer_id=?", direct)
		}
		if err = q.Order("id").Find(&campaigns).Error; err != nil {
			return err
		}
		instant, err := s.BusinessInstant(ctx, tx)
		if err != nil {
			return err
		}
		out = []ActivityOffer{}
		for _, c := range campaigns {
			var v campaignVersionRow
			if err = tx.First(&v, "campaign_id=? AND version=?", c.ID, c.CurrentVersion).Error; err != nil {
				return err
			}
			if v.PackageID != packageID || !v.Enabled || instant.At.Before(v.StartsAt) || !instant.At.Before(v.EndsAt) {
				continue
			}
			selection := ActivitySelection{}
			if c.Kind == "merchant" {
				selection.MerchantCampaignID = c.ID
			} else {
				selection.KOLCampaignID = c.ID
			}
			b, _ := json.Marshal(selection)
			_, previewErr := s.PreviewQuote(ctx, tx, customer, operator, *p.CurrentQuoteID, b)
			name := v.Name
			if name == "" {
				name = p.Name
			}
			offer := ActivityOffer{Name: name, ID: c.ID, Kind: c.Kind, OwnerCustomerID: c.OwnerCustomerID, PackageID: v.PackageID, FinalPriceMoney: v.FinalPriceMoney, GiftATG: v.GiftATG, ConcessionRate: v.ConcessionRate, StartsAt: v.StartsAt, EndsAt: v.EndsAt, PerCustomerLimit: v.PerCustomerLimit, Eligible: previewErr == nil}
			if previewErr != nil {
				offer.Reason = previewErr.Error()
			}
			out = append(out, offer)
		}
		// Combination is an explicit campaign pair, not a global promise that
		// any merchant activity can be combined with any KOL activity.
		var pairs []struct{ MerchantCampaignID, KOLCampaignID string }
		if err = tx.Table("incentives_combinations").Where("operator_id=?", operator).Find(&pairs).Error; err != nil {
			return err
		}
		visible := map[string]bool{}
		for _, o := range out {
			visible[o.ID] = true
		}
		for i := range out {
			out[i].CompatibleCampaignIDs = []string{}
			for _, pair := range pairs {
				other := ""
				if pair.MerchantCampaignID == out[i].ID {
					other = pair.KOLCampaignID
				} else if pair.KOLCampaignID == out[i].ID {
					other = pair.MerchantCampaignID
				}
				if visible[other] {
					out[i].CompatibleCampaignIDs = append(out[i].CompatibleCampaignIDs, other)
				}
			}
			out[i].AllowCombination = len(out[i].CompatibleCampaignIDs) > 0
		}
		return nil
	})
	return
}

type ManagedCampaign struct {
	Campaign Campaign        `json:"campaign"`
	Version  CampaignVersion `json:"version"`
}

func (s *Service) OwnCampaigns(ctx context.Context, tx *gorm.DB, operator, customer string) ([]ManagedCampaign, error) {
	own, err := s.org.CurrentOwnership(ctx, tx, customer)
	if err != nil {
		return nil, err
	}
	if own.OperatorID != operator {
		return nil, ErrNotEligible
	}
	var rows []campaignRow
	if err = s.dbFor(ctx, tx).Where("operator_id=? AND owner_customer_id=? AND kind='kol'", operator, customer).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	return s.managedCampaigns(s.dbFor(ctx, tx), rows)
}
func (s *Service) CampaignsForOperator(ctx context.Context, tx *gorm.DB, actor, operator string) ([]ManagedCampaign, error) {
	if err := s.org.RequirePermission(ctx, tx, actor, operator, "marketing.manage"); err != nil {
		return nil, err
	}
	var rows []campaignRow
	if err := s.dbFor(ctx, tx).Where("operator_id=?", operator).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	return s.managedCampaigns(s.dbFor(ctx, tx), rows)
}
func (s *Service) managedCampaigns(tx *gorm.DB, rows []campaignRow) ([]ManagedCampaign, error) {
	out := make([]ManagedCampaign, 0, len(rows))
	for _, r := range rows {
		var version campaignVersionRow
		if err := tx.First(&version, "campaign_id=? AND version=?", r.ID, r.CurrentVersion).Error; err != nil {
			return nil, err
		}
		if version.Name == "" {
			if err := tx.Table("commerce_packages").Select("name").Where("id=?", version.PackageID).Scan(&version.Name).Error; err != nil {
				return nil, err
			}
		}
		out = append(out, ManagedCampaign{Campaign: Campaign(r), Version: CampaignVersion(version)})
	}
	return out, nil
}

type PromotionOverview struct {
	Qualification           Qualification  `json:"qualification"`
	FunctionState           FunctionState  `json:"function_state"`
	DirectInvites           int64          `json:"direct_invites"`
	IndirectInvites         int64          `json:"indirect_invites"`
	FrozenMoney             ledger.Decimal `json:"frozen_money"`
	AvailableMoney          ledger.Decimal `json:"available_money"`
	HeldMoney               ledger.Decimal `json:"held_money"`
	PendingConsumptionMoney ledger.Decimal `json:"pending_consumption_money"`
	PendingCorrectionMoney  ledger.Decimal `json:"pending_correction_money"`
}

// FunctionStateForOperator is a read projection. Unlike PromotionOverview it
// neither advances qualification nor creates ledger accounts, so it is safe in
// the operator's repeatable-read customer workspace.
func (s *Service) FunctionStateForOperator(ctx context.Context, tx *gorm.DB, actor, operator, customer string) (FunctionState, error) {
	if err := s.org.RequirePermission(ctx, tx, actor, operator, "marketing.manage"); err != nil {
		return FunctionState{}, err
	}
	own, err := s.org.CurrentOwnership(ctx, tx, customer)
	if err != nil {
		return FunctionState{}, err
	}
	if own.OperatorID != operator {
		return FunctionState{}, ErrNotEligible
	}
	return s.stateAt(s.dbFor(ctx, tx), operator, customer, time.Now().UTC())
}

func (s *Service) PromotionOverview(ctx context.Context, tx *gorm.DB, operator, customer string) (out PromotionOverview, err error) {
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
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
		out.Qualification, err = s.Advance(ctx, tx, operator, customer, instant.At)
		if err != nil {
			return err
		}
		out.FunctionState, err = s.stateAt(tx, operator, customer, instant.At)
		if err != nil {
			return err
		}
		out.DirectInvites, out.IndirectInvites, err = s.org.ReferralCounts(ctx, tx, customer)
		if err != nil {
			return err
		}
		frozen, err := s.account(ctx, tx, operator, customer, "commission:frozen")
		if err != nil {
			return err
		}
		available, err := s.account(ctx, tx, operator, customer, "commission:available")
		if err != nil {
			return err
		}
		out.FrozenMoney = frozen.Balance
		out.AvailableMoney = available.Available()
		out.HeldMoney = available.Held
		if err = tx.Raw(`SELECT COALESCE(sum(CASE WHEN e.kind IN ('usage','expiry') THEN e.paid_money WHEN original.status IN ('pending','disputed') THEN e.paid_money ELSE 0 END),0)::text FROM incentives_confirmation_events e LEFT JOIN incentives_confirmation_events original ON original.id=e.original_event_id WHERE e.operator_id=? AND e.customer_id=? AND e.status IN ('pending','disputed')`, operator, customer).Row().Scan(&out.PendingConsumptionMoney); err != nil {
			return err
		}
		if err = tx.Raw(`SELECT COALESCE(sum(-paid_money),0)::text FROM incentives_confirmation_events WHERE operator_id=? AND customer_id=? AND kind='correction' AND status IN ('pending','disputed')`, operator, customer).Row().Scan(&out.PendingCorrectionMoney); err != nil {
			return err
		}
		return nil
	})
	return
}
func (s *Service) Withdrawals(ctx context.Context, tx *gorm.DB, operator, customer string) ([]Withdrawal, error) {
	own, err := s.org.CurrentOwnership(ctx, tx, customer)
	if err != nil {
		return nil, err
	}
	if own.OperatorID != operator {
		return nil, ErrNotEligible
	}
	var rows []withdrawalRow
	if err = s.dbFor(ctx, tx).Where("operator_id=? AND customer_id=?", operator, customer).Order("created_at DESC,id DESC").Limit(200).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Withdrawal, len(rows))
	for i, r := range rows {
		out[i] = Withdrawal(r)
	}
	return out, nil
}
