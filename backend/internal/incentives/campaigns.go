package incentives

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func (s *Service) PublishCampaign(ctx context.Context, tx *gorm.DB, in CampaignInput) (CampaignVersion, error) {
	return s.publishCampaign(ctx, tx, in, false)
}
func (s *Service) PublishKOLCampaign(ctx context.Context, tx *gorm.DB, customer string, in CampaignInput) (CampaignVersion, error) {
	in.Kind = "kol"
	in.OwnerCustomerID = customer
	in.ActorStaffID = ""
	return s.publishCampaign(ctx, tx, in, true)
}
func (s *Service) publishCampaign(ctx context.Context, tx *gorm.DB, in CampaignInput, asCustomer bool) (out CampaignVersion, err error) {
	if !validID(in.OperatorID) || !validID(in.PackageID) || (in.Kind != "merchant" && in.Kind != "kol") || (in.Eligibility != "any" && in.Eligibility != "first_order" && in.Eligibility != "existing") || !in.EndsAt.After(in.StartsAt) || in.PerCustomerLimit <= 0 || in.TotalUseLimit < 0 || in.GiftATG.Sign() < 0 || !in.GiftATG.Valid() || !rateValid(in.ConcessionRate) || utf8.RuneCountInString(strings.TrimSpace(in.Name)) > 100 {
		return out, ErrInvalidInput
	}
	if in.FinalPriceMoney != nil && (in.FinalPriceMoney.Sign() <= 0 || !in.FinalPriceMoney.Valid()) {
		return out, ErrInvalidInput
	}
	if in.Kind == "kol" && (!validID(in.OwnerCustomerID) || in.FinalPriceMoney != nil || !in.GiftATG.IsZero() || in.ConcessionRate.Sign() <= 0) {
		return out, ErrInvalidInput
	}
	if in.Kind == "merchant" && (!in.ConcessionRate.IsZero() || in.OwnerCustomerID != "") {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		if !asCustomer {
			if err := s.org.RequirePermission(ctx, tx, in.ActorStaffID, in.OperatorID, "marketing.manage"); err != nil {
				return err
			}
		}
		if err := s.LockBusiness(ctx, tx, in.OperatorID); err != nil {
			return err
		}
		instant, err := s.BusinessInstant(ctx, tx)
		if err != nil {
			return err
		}
		policy, err := s.policyAt(tx, in.OperatorID, instant.At)
		if err != nil {
			return err
		}
		p, err := s.commerce.GetPackage(ctx, tx, in.OperatorID, in.PackageID)
		if err != nil {
			return err
		}
		in.Name = strings.TrimSpace(in.Name)
		if in.Name == "" {
			in.Name = p.Name
		}
		if in.Kind == "merchant" && in.Enabled {
			if in.FinalPriceMoney == nil && in.GiftATG.IsZero() {
				return ErrInvalidInput
			}
			if in.FinalPriceMoney != nil {
				if p.CurrentQuoteID == nil {
					return ErrNotReady
				}
				quote, err := s.commerce.GetQuote(ctx, tx, *p.CurrentQuoteID)
				if err != nil {
					return err
				}
				if in.FinalPriceMoney.Cmp(quote.PriceMoney) > 0 {
					return ErrInvalidInput
				}
			}
		}
		if in.Kind == "kol" {
			if _, err = s.eligibleKOL(ctx, tx, in.OperatorID, in.OwnerCustomerID, instant.At); err != nil {
				return err
			}
			if in.ConcessionRate.Cmp(policy.DirectRate) > 0 {
				return ErrNotEligible
			}
		}
		var campaign campaignRow
		if in.CampaignID == "" {
			if in.ExpectedVersion != 0 {
				return ErrConflict
			}
			campaign = campaignRow{ID: uuid.NewString(), OperatorID: in.OperatorID, Kind: in.Kind}
			if in.Kind == "kol" {
				v := in.OwnerCustomerID
				campaign.OwnerCustomerID = &v
			}
			if err = tx.Create(&campaign).Error; err != nil {
				return err
			}
		} else {
			if !validID(in.CampaignID) {
				return ErrInvalidInput
			}
			if err = tx.First(&campaign, "id=?", in.CampaignID).Error; err != nil {
				return err
			}
			if campaign.OperatorID != in.OperatorID || campaign.Kind != in.Kind || campaign.CurrentVersion != in.ExpectedVersion {
				return ErrConflict
			}
			if in.Kind == "kol" && (campaign.OwnerCustomerID == nil || *campaign.OwnerCustomerID != in.OwnerCustomerID) {
				return ErrNotEligible
			}
		}
		var before any
		if campaign.CurrentVersion > 0 {
			var previous campaignVersionRow
			if err = tx.First(&previous, "campaign_id=? AND version=?", campaign.ID, campaign.CurrentVersion).Error; err != nil {
				return err
			}
			before = CampaignVersion(previous)
		}
		r := campaignVersionRow{Name: in.Name, ID: uuid.NewString(), CampaignID: campaign.ID, PackageID: in.PackageID, Version: campaign.CurrentVersion + 1, Enabled: in.Enabled, StartsAt: in.StartsAt.UTC(), EndsAt: in.EndsAt.UTC(), Eligibility: in.Eligibility, FinalPriceMoney: in.FinalPriceMoney, GiftATG: in.GiftATG, ConcessionRate: in.ConcessionRate, PerCustomerLimit: in.PerCustomerLimit, TotalUseLimit: in.TotalUseLimit, CreatedAt: instant.At}
		if err = tx.Create(&r).Error; err != nil {
			return err
		}
		if err = tx.Model(&campaign).Update("current_version", r.Version).Error; err != nil {
			return err
		}
		out = CampaignVersion(r)
		actor := in.ActorStaffID
		if asCustomer {
			actor = in.OwnerCustomerID
		}
		return s.event(tx, in.OperatorID, campaign.ID, "campaign.version_published", map[string]any{"actor_id": actor, "evidence": in.Evidence, "before": before, "after": out})
	})
	return
}
func (s *Service) AllowCombination(ctx context.Context, tx *gorm.DB, actor, operator, merchant, kol string, enabled bool) error {
	if !validID(merchant) || !validID(kol) {
		return ErrInvalidInput
	}
	return s.within(ctx, tx, func(tx *gorm.DB) error {
		if err := s.org.RequirePermission(ctx, tx, actor, operator, "marketing.manage"); err != nil {
			return err
		}
		if err := s.LockBusiness(ctx, tx, operator); err != nil {
			return err
		}
		var rows []campaignRow
		if err := tx.Where("id IN ?", []string{merchant, kol}).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != 2 {
			return ErrNotFound
		}
		for _, r := range rows {
			if r.OperatorID != operator || (r.ID == merchant && r.Kind != "merchant") || (r.ID == kol && r.Kind != "kol") {
				return ErrNotEligible
			}
		}
		if enabled {
			var versions []campaignVersionRow
			for _, r := range rows {
				var v campaignVersionRow
				if err := tx.First(&v, "campaign_id=? AND version=?", r.ID, r.CurrentVersion).Error; err != nil {
					return err
				}
				versions = append(versions, v)
			}
			if versions[0].PackageID != versions[1].PackageID {
				return ErrNotEligible
			}
			return tx.Exec(`INSERT INTO incentives_combinations(operator_id,merchant_campaign_id,kol_campaign_id) VALUES(?,?,?) ON CONFLICT DO NOTHING`, operator, merchant, kol).Error
		}
		return tx.Exec(`DELETE FROM incentives_combinations WHERE operator_id=? AND merchant_campaign_id=? AND kol_campaign_id=?`, operator, merchant, kol).Error
	})
}
func (s *Service) eligibleKOL(ctx context.Context, tx *gorm.DB, operator, customer string, at time.Time) (Qualification, error) {
	own, err := s.org.CurrentOwnership(ctx, tx, customer)
	if err != nil {
		return Qualification{}, err
	}
	if own.OperatorID != operator {
		return Qualification{}, ErrNotEligible
	}
	if err = s.advance(ctx, tx, operator, customer, at); err != nil {
		return Qualification{}, err
	}
	q, err := s.ensureQualification(tx, operator, customer)
	if err != nil {
		return q, err
	}
	state, err := s.stateAt(tx, operator, customer, at)
	if err != nil {
		return q, err
	}
	if q.AcquiredAt == nil || q.AcquiredAt.After(at) || state.Status != "active" {
		return q, ErrNotEligible
	}
	return q, nil
}

// Quote implements commerce.Marketing, reserving activity places and recording gifts and discounts
// alongside the order and preserving the original two-hop identities/rates.
func (s *Service) Quote(ctx context.Context, tx *gorm.DB, o commerce.Order, selection json.RawMessage) (out commerce.MarketingQuote, err error) {
	if tx == nil {
		return out, ErrInvalidInput
	}
	var chosen ActivitySelection
	if len(selection) > 0 && string(selection) != "null" {
		decoder := json.NewDecoder(bytes.NewReader(selection))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&chosen); err != nil {
			return out, ErrInvalidInput
		}
		if err = decoder.Decode(&struct{}{}); err != io.EOF {
			return out, ErrInvalidInput
		}
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		if err := s.LockBusiness(ctx, tx, o.OperatorID); err != nil {
			return err
		}
		if chosen.MerchantCampaignID != "" || chosen.KOLCampaignID != "" {
			if err := s.org.RequireCustomerAction(ctx, tx, o.CustomerID, o.OperatorID, string(organization.Activity)); err != nil {
				return err
			}
			if err := s.org.RequireBusiness(ctx, tx, o.OperatorID, organization.Activity); err != nil {
				return err
			}
		}
		instant, err := s.BusinessInstant(ctx, tx)
		if err != nil {
			return err
		}
		policy, err := s.policyAt(tx, o.OperatorID, instant.At)
		if err != nil {
			return err
		}
		if o.CommissionMoney.Sign() > 0 {
			if err = s.advance(ctx, tx, o.OperatorID, o.CustomerID, instant.At); err != nil {
				return err
			}
			state, err := s.stateAt(tx, o.OperatorID, o.CustomerID, instant.At)
			if err != nil {
				return err
			}
			if state.ReleaseBlocked {
				return ErrBlocked
			}
		}
		if o.QuoteID == nil {
			return ErrInvalidInput
		}
		quote, err := s.commerce.GetQuote(ctx, tx, *o.QuoteID)
		if err != nil {
			return err
		}
		chain, err := s.org.ReferralChain(ctx, tx, o.CustomerID)
		if err != nil {
			return err
		}
		snap := snapshotRow{OrderID: o.ID, OperatorID: o.OperatorID, CustomerID: o.CustomerID, PolicyID: policy.ID, DirectRate: policy.DirectRate, IndirectRate: policy.IndirectRate, GiftRemainder: "0", Status: "reserved", CreatedAt: instant.At}
		if len(chain) > 0 {
			v := chain[0].InviterID
			snap.DirectCustomerID = &v
		}
		if len(chain) > 1 {
			v := chain[1].InviterID
			snap.IndirectCustomerID = &v
		}
		price := o.DueMoney
		if chosen.MerchantCampaignID != "" && chosen.KOLCampaignID != "" {
			var count int64
			if err = tx.Table("incentives_combinations").Where("operator_id=? AND merchant_campaign_id=? AND kol_campaign_id=?", o.OperatorID, chosen.MerchantCampaignID, chosen.KOLCampaignID).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return ErrCombination
			}
		}
		for _, selected := range []struct{ id, kind string }{{chosen.MerchantCampaignID, "merchant"}, {chosen.KOLCampaignID, "kol"}} {
			if selected.id == "" {
				continue
			}
			if !validID(selected.id) {
				return ErrInvalidInput
			}
			var c campaignRow
			var v campaignVersionRow
			if err = tx.First(&c, "id=?", selected.id).Error; err != nil {
				return err
			}
			if c.OperatorID != o.OperatorID || c.Kind != selected.kind {
				return ErrNotEligible
			}
			if err = tx.First(&v, "campaign_id=? AND version=?", c.ID, c.CurrentVersion).Error; err != nil {
				return err
			}
			if !v.Enabled || v.PackageID != quote.PackageID || instant.At.Before(v.StartsAt) || !instant.At.Before(v.EndsAt) {
				return ErrNotEligible
			}
			completed, err := s.commerce.CountCompletedOrders(ctx, tx, o.CustomerID)
			if err != nil {
				return err
			}
			if (v.Eligibility == "first_order" && completed != 0) || (v.Eligibility == "existing" && completed == 0) {
				return ErrNotEligible
			}
			gift, discount := v.GiftATG, ledger.Zero()
			if selected.kind == "merchant" {
				if v.FinalPriceMoney != nil {
					if v.FinalPriceMoney.Cmp(o.DueMoney) > 0 {
						return ErrNotEligible
					}
					price = *v.FinalPriceMoney
					discount = o.DueMoney.Sub(price)
				}
				snap.MerchantCampaignID = &c.ID
				snap.MerchantVersionID = &v.ID
				snap.MerchantGiftATG = gift
				snap.DiscountMoney = discount
			} else {
				if c.OwnerCustomerID == nil || snap.DirectCustomerID == nil || *c.OwnerCustomerID != *snap.DirectCustomerID {
					return ErrNotEligible
				}
				if _, err = s.eligibleKOL(ctx, tx, o.OperatorID, *c.OwnerCustomerID, instant.At); err != nil {
					return err
				}
				if v.ConcessionRate.Cmp(policy.DirectRate) > 0 {
					return ErrNotEligible
				}
				exact := new(big.Rat).Mul(o.PaidATG.Add(o.PackageGiftATG).Rat(), v.ConcessionRate.Rat())
				var remainder *big.Rat
				gift, remainder = ledger.Quantize(exact)
				snap.KOLGiftATG = gift
				snap.GiftRemainder = remainder.RatString()
				snap.ConcessionRate = v.ConcessionRate
				snap.KOLCampaignID = &c.ID
				snap.KOLVersionID = &v.ID
				snap.KOLCustomerID = c.OwnerCustomerID
				if err = s.reserveKOLCross(tx, o.OperatorID, *c.OwnerCustomerID, o.CustomerID, policy.KOLCrossActivityLimit); err != nil {
					return err
				}
			}
			if err = s.reserveCampaign(tx, Campaign(c), CampaignVersion(v), o.CustomerID, gift, discount); err != nil {
				return err
			}
		}
		if err = tx.Create(&snap).Error; err != nil {
			return err
		}
		b, err := json.Marshal(OrderSnapshot(snap))
		if err != nil {
			return err
		}
		out = commerce.MarketingQuote{FinalMoney: price, MerchantGiftATG: snap.MerchantGiftATG, KOLGiftATG: snap.KOLGiftATG, Reference: o.ID, CommissionSnapshot: b}
		return nil
	})
	return
}

func (s *Service) reserveCampaign(tx *gorm.DB, c Campaign, v CampaignVersion, customer string, gift, discount ledger.Decimal) error {
	if v.TotalUseLimit > 0 && c.ReservedUses+c.UsedUses+1 > v.TotalUseLimit {
		return ErrLimit
	}
	if err := tx.Exec(`INSERT INTO incentives_campaign_customer_counts(campaign_id,customer_id) VALUES(?,?) ON CONFLICT DO NOTHING`, c.ID, customer).Error; err != nil {
		return err
	}
	var counter struct{ Reserved, Used int64 }
	if err := tx.Table("incentives_campaign_customer_counts").Where("campaign_id=? AND customer_id=?", c.ID, customer).First(&counter).Error; err != nil {
		return err
	}
	if counter.Reserved+counter.Used+1 > v.PerCustomerLimit {
		return ErrLimit
	}
	if err := tx.Exec(`UPDATE incentives_campaign_customer_counts SET reserved=reserved+1 WHERE campaign_id=? AND customer_id=?`, c.ID, customer).Error; err != nil {
		return err
	}
	return tx.Model(&campaignRow{}).Where("id=?", c.ID).Updates(map[string]any{"reserved_uses": c.ReservedUses + 1, "reserved_gift_atg": c.ReservedGiftATG.Add(gift), "reserved_discount_money": c.ReservedDiscountMoney.Add(discount)}).Error
}
func (s *Service) reserveKOLCross(tx *gorm.DB, operator, kol, customer string, limit int) error {
	if err := tx.Exec(`INSERT INTO incentives_kol_customer_counts(operator_id,kol_customer_id,customer_id) VALUES(?,?,?) ON CONFLICT DO NOTHING`, operator, kol, customer).Error; err != nil {
		return err
	}
	var counter struct{ Reserved, Used int64 }
	if err := tx.Table("incentives_kol_customer_counts").Where("operator_id=? AND kol_customer_id=? AND customer_id=?", operator, kol, customer).First(&counter).Error; err != nil {
		return err
	}
	if limit > 0 && counter.Reserved+counter.Used+1 > int64(limit) {
		return ErrLimit
	}
	return tx.Exec(`UPDATE incentives_kol_customer_counts SET reserved=reserved+1 WHERE operator_id=? AND kol_customer_id=? AND customer_id=?`, operator, kol, customer).Error
}

func (s *Service) Fulfill(ctx context.Context, tx *gorm.DB, o commerce.Order) error {
	return s.resolveCampaigns(ctx, tx, o, true)
}
func (s *Service) Release(ctx context.Context, tx *gorm.DB, o commerce.Order) error {
	return s.resolveCampaigns(ctx, tx, o, false)
}
func (s *Service) resolveCampaigns(ctx context.Context, tx *gorm.DB, o commerce.Order, fulfill bool) error {
	if tx == nil {
		return ErrInvalidInput
	}
	return s.within(ctx, tx, func(tx *gorm.DB) error {
		if err := s.LockBusiness(ctx, tx, o.OperatorID); err != nil {
			return err
		}
		var snap snapshotRow
		if err := tx.First(&snap, "order_id=?", o.ID).Error; err != nil {
			return err
		}
		if snap.OperatorID != o.OperatorID || snap.CustomerID != o.CustomerID {
			return ErrNotEligible
		}
		target := "released"
		if fulfill {
			target = "fulfilled"
		}
		if snap.Status == target {
			return nil
		}
		if snap.Status == "fulfilled" {
			return ErrConflict
		}
		restored := snap.Status == "released" && fulfill
		for _, item := range []struct {
			id             *string
			gift, discount ledger.Decimal
		}{{snap.MerchantCampaignID, snap.MerchantGiftATG, snap.DiscountMoney}, {snap.KOLCampaignID, snap.KOLGiftATG, ledger.Zero()}} {
			if item.id == nil {
				continue
			}
			reservedDelta, usedDelta := int64(-1), int64(0)
			giftReserved, discountReserved := item.gift.Neg(), item.discount.Neg()
			if fulfill {
				usedDelta = 1
			}
			if restored {
				reservedDelta = 0
				giftReserved = ledger.Zero()
				discountReserved = ledger.Zero()
			}
			giftUsed, discountUsed := ledger.Zero(), ledger.Zero()
			if fulfill {
				giftUsed = item.gift
				discountUsed = item.discount
			}
			if err := tx.Exec(`UPDATE incentives_campaigns SET reserved_uses=reserved_uses+?,used_uses=used_uses+?,reserved_gift_atg=reserved_gift_atg+?,used_gift_atg=used_gift_atg+?,reserved_discount_money=reserved_discount_money+?,used_discount_money=used_discount_money+? WHERE id=?`, reservedDelta, usedDelta, giftReserved, giftUsed, discountReserved, discountUsed, *item.id).Error; err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE incentives_campaign_customer_counts SET reserved=reserved+?,used=used+? WHERE campaign_id=? AND customer_id=?`, reservedDelta, usedDelta, *item.id, o.CustomerID).Error; err != nil {
				return err
			}
		}
		if snap.KOLCustomerID != nil {
			rd, ud := int64(-1), int64(0)
			if fulfill {
				ud = 1
			}
			if restored {
				rd = 0
			}
			if err := tx.Exec(`UPDATE incentives_kol_customer_counts SET reserved=reserved+?,used=used+? WHERE operator_id=? AND kol_customer_id=? AND customer_id=?`, rd, ud, o.OperatorID, *snap.KOLCustomerID, o.CustomerID).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&snap).Where("order_id=?", o.ID).Update("status", target).Error; err != nil {
			return err
		}
		kind := "campaign.commitment_" + target
		if restored {
			kind = "campaign.late_commitment_restored"
		}
		return s.event(tx, o.OperatorID, o.ID, kind, map[string]any{"order_id": o.ID})
	})
}
