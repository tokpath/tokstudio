package remediation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrTakeoverBlocked = errors.New("channel takeover awaits outstanding operations or refreshed approval")

type TakeoverInput struct {
	ActorID                string         `json:"actor_id"`
	ChannelID              string         `json:"channel_id"`
	BrandOperatorID        string         `json:"brand_operator_id"`
	BusinessKey            string         `json:"business_key"`
	Method                 string         `json:"method"`
	ServiceMoneyPerATG     ledger.Decimal `json:"service_money_per_atg"`
	ProcurementMoneyPerATG ledger.Decimal `json:"procurement_money_per_atg"`
	ExtraMoney             ledger.Decimal `json:"extra_money"`
	ContractReference      string         `json:"contract_reference"`
	Evidence               string         `json:"evidence"`
}
type Takeover struct {
	ID                              string          `json:"id"`
	ChannelID                       string          `json:"channel_id"`
	BrandOperatorID                 string          `json:"brand_operator_id"`
	BusinessKey                     string          `json:"business_key"`
	Fingerprint                     string          `json:"-"`
	State                           string          `json:"state"`
	Method                          string          `json:"method"`
	ServiceMoneyPerATG              ledger.Decimal  `json:"service_money_per_atg"`
	ProcurementMoneyPerATG          ledger.Decimal  `json:"procurement_money_per_atg"`
	ExtraMoney                      ledger.Decimal  `json:"extra_money"`
	ContractReference               string          `json:"contract_reference"`
	Evidence                        string          `json:"evidence"`
	RequestedBy                     string          `json:"requested_by"`
	ApprovedBy                      *string         `json:"approved_by,omitempty"`
	PreviousStatus                  json.RawMessage `gorm:"type:jsonb" json:"-"`
	StatusVersion                   int64           `json:"status_version"`
	Snapshot                        json.RawMessage `gorm:"type:jsonb" json:"snapshot"`
	SnapshotFingerprint             string          `json:"snapshot_fingerprint"`
	ServiceMoney                    ledger.Decimal  `json:"service_money"`
	CashMoney                       ledger.Decimal  `json:"cash_money"`
	ProcurementOffsetATG            ledger.Decimal  `json:"procurement_offset_atg"`
	ProcurementOffsetMoney          ledger.Decimal  `json:"procurement_offset_money"`
	RemainingProcurementRefundMoney ledger.Decimal  `json:"remaining_procurement_refund_money"`
	SettlementDueMoney              ledger.Decimal  `json:"settlement_due_money"`
	SettledMoney                    ledger.Decimal  `json:"settled_money"`
	CreatedAt                       time.Time       `json:"created_at"`
	CompletedAt                     *time.Time      `json:"completed_at,omitempty"`
}

func (Takeover) TableName() string { return "remediation_takeovers" }

type TakeoverAsset struct {
	AccountID  string         `json:"account_id"`
	CustomerID string         `json:"customer_id"`
	Purpose    string         `json:"purpose"`
	Unit       string         `json:"unit"`
	Balance    ledger.Decimal `json:"balance"`
	Held       ledger.Decimal `json:"held"`
	Version    int64          `json:"version"`
}
type TakeoverSnapshot struct {
	Customers           map[string]int64                `json:"customers"`
	Assets              []TakeoverAsset                 `json:"assets"`
	Coverage            cooperation.CoverageSnapshot    `json:"coverage"`
	Procurement         cooperation.ProcurementPosition `json:"procurement"`
	Lots                []cooperation.ProcurementLot    `json:"lots"`
	ServiceATG          ledger.Decimal                  `json:"service_atg"`
	ServiceMoney        ledger.Decimal                  `json:"service_money"`
	CashMoney           ledger.Decimal                  `json:"cash_money"`
	OffsetATG           ledger.Decimal                  `json:"offset_atg"`
	OffsetMoney         ledger.Decimal                  `json:"offset_money"`
	ResidualRefundMoney ledger.Decimal                  `json:"residual_refund_money"`
	SettlementDueMoney  ledger.Decimal                  `json:"settlement_due_money"`
	RoundingEvidence    map[string]string               `json:"rounding_evidence"`
}

// BeginTakeover records an explicit brand decision, pauses the channel's new
// business and allows accepted work to settle. It never turns a short procurement
// shortage into an automatic, unapproved seizure of a channel's business.
func (s *Service) BeginTakeover(ctx context.Context, in TakeoverInput) (Takeover, error) {
	var out Takeover
	if !validID(in.ChannelID) || !validID(in.BrandOperatorID) || in.BusinessKey == "" || len(in.BusinessKey) > 150 || in.ContractReference == "" || in.Evidence == "" || !validAmount(in.ServiceMoneyPerATG) || !validAmount(in.ProcurementMoneyPerATG) || !in.ExtraMoney.Valid() || (in.Method != "supply_coverage" && in.Method != "agreed_atg_value") {
		return out, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, in.ActorID, in.BrandOperatorID, "cooperation.manage"); e != nil {
			return e
		}
		if e := s.Organization.RequirePermission(ctx, t, in.ActorID, in.BrandOperatorID, "refunds.manage"); e != nil {
			return e
		}
		if e := s.lockTakeoverOperators(t, in.ChannelID, in.BrandOperatorID); e != nil {
			return e
		}
		channel, e := s.Organization.GetOperator(ctx, t, in.ChannelID)
		if e != nil {
			return e
		}
		brand, e := s.Organization.GetBrand(ctx, t, channel.BrandID)
		if e != nil {
			return e
		}
		if channel.Kind != organization.Channel || channel.ParentOperatorID == nil || *channel.ParentOperatorID != in.BrandOperatorID || brand.OwnerOperatorID != in.BrandOperatorID {
			return ErrScope
		}
		e = t.First(&out, "channel_id=? AND business_key=?", in.ChannelID, in.BusinessKey).Error
		if e == nil {
			if out.Fingerprint != fingerprint(in) {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		var n int64
		if e = t.Model(&Takeover{}).Where("channel_id=? AND state<>'cancelled'", in.ChannelID).Count(&n).Error; e != nil {
			return e
		}
		if n > 0 {
			return ErrState
		}
		status, e := s.Organization.GetBusinessStatus(ctx, t, in.ChannelID)
		if e != nil {
			return e
		}
		previous, _ := json.Marshal(status)
		at, e := instant(t)
		if e != nil {
			return e
		}
		out = Takeover{ID: uuid.NewString(), ChannelID: in.ChannelID, BrandOperatorID: in.BrandOperatorID, BusinessKey: in.BusinessKey, Fingerprint: fingerprint(in), State: "preparing", Method: in.Method, ServiceMoneyPerATG: in.ServiceMoneyPerATG, ProcurementMoneyPerATG: in.ProcurementMoneyPerATG, ExtraMoney: in.ExtraMoney, ContractReference: in.ContractReference, Evidence: in.Evidence, RequestedBy: in.ActorID, PreviousStatus: previous, StatusVersion: status.Version + 1, CreatedAt: at}
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		if e = t.Table("organization_operator_statuses").Where("operator_id=? AND version=?", in.ChannelID, status.Version).Updates(map[string]any{"registration": false, "new_purchase": false, "renewal": false, "model_usage": false, "activity": false, "version": status.Version + 1, "updated_at": at}).Error; e != nil {
			return e
		}
		if e = s.Cooperation.SuspendReplenishment(ctx, t, in.ChannelID, "takeover:"+out.ID); e != nil {
			return e
		}
		return s.event(t, in.BrandOperatorID, out.ID, "takeover.preparing", in.ActorID, in.Evidence, out)
	})
	return out, e
}
func (s *Service) lockTakeoverOperators(t *gorm.DB, from, to string) error {
	ids := []string{from, to}
	sort.Strings(ids)
	var rows []struct{ ID string }
	return t.Table("organization_operators").Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id IN ?", ids).Order("id").Find(&rows).Error
}
func (s *Service) lockTakeover(ctx context.Context, t *gorm.DB, actor, brand, id string) (Takeover, error) {
	var out Takeover
	if e := s.Organization.RequirePermission(ctx, t, actor, brand, "cooperation.manage"); e != nil {
		return out, e
	}
	if e := t.First(&out, "id=? AND brand_operator_id=?", id, brand).Error; e != nil {
		return out, e
	}
	if e := s.lockTakeoverOperators(t, out.ChannelID, out.BrandOperatorID); e != nil {
		return out, e
	}
	e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&out, "id=?", id).Error
	return out, e
}
func (s *Service) takeoverSnapshot(ctx context.Context, t *gorm.DB, p Takeover) (TakeoverSnapshot, error) {
	out := TakeoverSnapshot{Customers: map[string]int64{}, Assets: []TakeoverAsset{}, Lots: []cooperation.ProcurementLot{}, RoundingEvidence: map[string]string{}}
	// Raw, verified money can exist without a USD receipt after the checkout
	// expired. It must be valued and assigned before the channel is treated as
	// settled; a closed commercial order cannot erase the real payment.
	if e := s.requireTakeoverReceiptSettlement(ctx, t, p.ChannelID); e != nil {
		return out, e
	}
	// This precedes every ledger and incentive lock, including ordinary expiry.
	// Operator fences prevent new registrations while the complete set is valued.
	if e := s.Cooperation.Lock(ctx, t, p.ChannelID); e != nil {
		return out, e
	}
	if e := s.Cooperation.Lock(ctx, t, p.BrandOperatorID); e != nil {
		return out, e
	}
	for _, operator := range []string{p.ChannelID, p.BrandOperatorID} {
		if e := s.Incentives.LockBusiness(ctx, t, operator); e != nil {
			return out, e
		}
	}
	var owns []struct {
		CustomerID string
		Version    int64
	}
	if e := t.Table("organization_service_ownerships").Clauses(clause.Locking{Strength: "UPDATE"}).Select("customer_id,version").Where("operator_id=? AND ended_at IS NULL", p.ChannelID).Order("customer_id").Find(&owns).Error; e != nil {
		return out, e
	}
	for _, own := range owns {
		out.Customers[own.CustomerID] = own.Version
	}
	transfer := organization.ResponsibilityTransferInput{FromOperatorID: p.ChannelID, ToOperatorID: p.BrandOperatorID, TakeoverID: p.ID, ActorID: p.RequestedBy, Evidence: p.Evidence, Customers: out.Customers}
	if e := s.Commerce.PreflightTransferResponsibility(ctx, t, transfer); e != nil {
		return out, e
	}
	if e := s.Incentives.PreflightTransferResponsibility(ctx, t, transfer); e != nil {
		return out, e
	}
	for _, spec := range []struct{ table, where string }{{"usage_requests", "operator_id=? AND status IN ('accepted','executing','submitted','awaiting_usage')"}, {"remediation_cases", "operator_id=? AND state NOT IN ('completed','cancelled','rejected')"}, {"remediation_chargebacks", "operator_id=? AND state='disputed'"}, {"cooperation_purchase_orders", "buyer_id=? AND status NOT IN ('completed','closed')"}} {
		var n int64
		if e := t.Table(spec.table).Where(spec.where, p.ChannelID).Count(&n).Error; e != nil {
			return out, e
		}
		if n > 0 {
			return out, fmt.Errorf("%w: %s has %d unfinished operations", ErrTakeoverBlocked, spec.table, n)
		}
	}
	pendingProcurement, e := s.Cooperation.PendingProcurement(ctx, t, p.ChannelID)
	if e != nil {
		return out, e
	}
	if pendingProcurement.AutomaticAttempts+pendingProcurement.PurchaseOrders+pendingProcurement.Refunds > 0 {
		return out, ErrTakeoverBlocked
	}
	at, e := instant(t)
	if e != nil {
		return out, e
	}
	ids := []string{}
	for id := range out.Customers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	// Expiry is an ordinary source event, not a takeover loss. Finish its
	// attribution before valuing and moving the remaining service obligation.
	for _, id := range ids {
		events, e := s.Ledger.ExpireCredits(ctx, t, p.ChannelID, id, at)
		if e != nil {
			return out, e
		}
		for _, ev := range events {
			g, e := s.Ledger.GetGrant(ctx, t, ev.GrantID)
			if e != nil {
				return out, e
			}
			if e = s.Incentives.ObserveGrantEvent(ctx, t, g, ev); e != nil {
				return out, e
			}
		}
	}
	if len(ids) > 0 {
		if e = t.Table("ledger_accounts").Select("id AS account_id,owner_id AS customer_id,purpose,unit,balance,held,version").Where("operator_id=? AND owner_id IN ?", p.ChannelID, ids).Order("owner_id,purpose,id").Find(&out.Assets).Error; e != nil {
			return out, e
		}
	}
	for _, a := range out.Assets {
		if a.Unit == "MONEY" && (a.Purpose == "customer:money" || a.Purpose == "commission:available" || a.Purpose == "commission:frozen") {
			out.CashMoney = out.CashMoney.Add(a.Balance)
		}
	}
	if p.Method == "supply_coverage" {
		out.Coverage, e = s.Cooperation.Coverage(ctx, t, p.ChannelID, at)
		if e != nil {
			return out, e
		}
		out.ServiceATG = out.Coverage.Required.Add(out.Coverage.Pending)
	} else {
		// A contract can specify a fixed handover value even when the channel's
		// model supply has already stopped. It does not require a usable price book.
		out.Coverage = cooperation.CoverageSnapshot{OperatorID: p.ChannelID, HasUpstream: true, Status: "contract_valuation"}
		out.ServiceATG, e = s.Ledger.UnheldLiability(ctx, t, p.ChannelID, at)
		if e != nil {
			return out, e
		}
		var pending ledger.Decimal
		if e = t.Table("cooperation_sales_commitments").Select("COALESCE(sum(atg),0)::text").Where("operator_id=? AND status='active'", p.ChannelID).Row().Scan(&pending); e != nil {
			return out, e
		}
		out.ServiceATG = out.ServiceATG.Add(pending)
	}
	var tail *big.Rat
	out.ServiceMoney, tail = ledger.Quantize(new(big.Rat).Mul(out.ServiceATG.Rat(), p.ServiceMoneyPerATG.Rat()))
	out.RoundingEvidence["service_money"] = tail.RatString()
	position, e := s.Cooperation.Position(ctx, t, p.ChannelID)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		position = cooperation.ProcurementPosition{BuyerID: p.ChannelID}
	} else if e != nil {
		return out, e
	}
	out.Procurement = position
	if position.Quantity.Sign() > 0 {
		account, e := s.Ledger.AccountInTransaction(ctx, t, position.AccountID)
		if e != nil {
			return out, e
		}
		if account.Held.Sign() > 0 {
			return out, ErrTakeoverBlocked
		}
		if e = t.Where("buyer_id=? AND remaining>0", p.ChannelID).Order("delivered_at,id").Find(&out.Lots).Error; e != nil {
			return out, e
		}
	}
	offset, tail := ledger.Quantize(new(big.Rat).Quo(out.ServiceMoney.Rat(), p.ProcurementMoneyPerATG.Rat()))
	out.RoundingEvidence["offset_atg"] = tail.RatString()
	out.OffsetATG = min(position.Quantity, offset)
	out.OffsetMoney, tail = ledger.Quantize(new(big.Rat).Mul(out.OffsetATG.Rat(), p.ProcurementMoneyPerATG.Rat()))
	out.RoundingEvidence["offset_money"] = tail.RatString()
	left := out.OffsetATG
	for _, lot := range out.Lots {
		paid := lot.PaidATG.Sub(lot.PaidConsumed).Sub(lot.PaidRemoved)
		gift := lot.GiftATG.Sub(lot.GiftConsumed).Sub(lot.GiftRemoved)
		used := min(left, lot.Remaining)
		parts, e := ledger.Proportional(used, []ledger.Decimal{paid, gift})
		if e != nil {
			return out, e
		}
		left = left.Sub(used)
		remainingPaid := paid.Sub(parts[0])
		if remainingPaid.Sign() > 0 {
			terms, e := cooperation.ParseProcurementExitTerms(lot.ExitTerms)
			if e != nil {
				return out, e
			}
			if terms.VoluntaryRefund {
				value, tail := ledger.Quantize(new(big.Rat).Quo(new(big.Rat).Mul(lot.PaidMoney.Rat(), remainingPaid.Rat()), lot.PaidATG.Rat()))
				out.ResidualRefundMoney = out.ResidualRefundMoney.Add(value)
				out.RoundingEvidence["lot_refund_money:"+lot.ID] = tail.RatString()
			}
		}
	}
	out.SettlementDueMoney = out.ServiceMoney.Sub(out.OffsetMoney).Add(out.CashMoney).Add(p.ExtraMoney).Sub(out.ResidualRefundMoney)
	for _, v := range []ledger.Decimal{out.ServiceMoney, out.CashMoney, out.OffsetATG, out.OffsetMoney, out.ResidualRefundMoney, out.SettlementDueMoney} {
		if !v.Valid() {
			return out, ledger.ErrAmount
		}
	}
	return out, nil
}
func (s *Service) RefreshTakeover(ctx context.Context, actor, brand, id string) (Takeover, error) {
	var out Takeover
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		var e error
		out, e = s.lockTakeover(ctx, t, actor, brand, id)
		if e != nil {
			return e
		}
		if out.State != "preparing" && out.State != "awaiting_approval" {
			return ErrState
		}
		snapshot, e := s.takeoverSnapshot(ctx, t, out)
		if e != nil {
			return fmt.Errorf("takeover snapshot: %w", e)
		}
		out.Snapshot, _ = json.Marshal(snapshot)
		out.SnapshotFingerprint = fingerprint(snapshot)
		out.ServiceMoney = snapshot.ServiceMoney
		out.CashMoney = snapshot.CashMoney
		out.ProcurementOffsetATG = snapshot.OffsetATG
		out.ProcurementOffsetMoney = snapshot.OffsetMoney
		out.RemainingProcurementRefundMoney = snapshot.ResidualRefundMoney
		out.SettlementDueMoney = snapshot.SettlementDueMoney
		out.State = "awaiting_approval"
		return t.Save(&out).Error
	})
	return out, e
}

type takeoverRemovalGuard struct {
	s    *Service
	plan Takeover
}

func (g takeoverRemovalGuard) RequireProcurementRemoval(ctx context.Context, t *gorm.DB, buyer string, q ledger.Decimal, mode, evidence string) error {
	if mode != "takeover" || buyer != g.plan.ChannelID || evidence != g.plan.ContractReference {
		return ErrScope
	}
	var p Takeover
	if e := t.First(&p, "id=? AND state='executing'", g.plan.ID).Error; e != nil {
		return e
	}
	var n int64
	if e := t.Table("organization_service_ownerships").Where("operator_id=? AND ended_at IS NULL", buyer).Count(&n).Error; e != nil {
		return e
	}
	if n != 0 || p.ApprovedBy == nil {
		return ErrTakeoverBlocked
	}
	return nil
}
func (s *Service) ExecuteTakeover(ctx context.Context, actor, brand, id, expectedSnapshot string) (Takeover, error) {
	var out Takeover
	if expectedSnapshot == "" {
		return out, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		var e error
		out, e = s.lockTakeover(ctx, t, actor, brand, id)
		if e != nil {
			return e
		}
		if e = s.Organization.RequirePermission(ctx, t, actor, brand, "refunds.manage"); e != nil {
			return e
		}
		if out.State == "completed" {
			if out.SnapshotFingerprint != expectedSnapshot {
				return ErrConflict
			}
			return nil
		}
		if out.State != "awaiting_approval" || out.SnapshotFingerprint != expectedSnapshot {
			return ErrTakeoverBlocked
		}
		snapshot, e := s.takeoverSnapshot(ctx, t, out)
		if e != nil {
			return fmt.Errorf("takeover execution snapshot: %w", e)
		}
		if fingerprint(snapshot) != expectedSnapshot {
			return ErrTakeoverBlocked
		}
		reviews, e := s.reviews(t, brand, out.ServiceMoney.Add(out.CashMoney))
		if e != nil {
			return e
		}
		if reviews > 1 && actor == out.RequestedBy {
			return ErrReview
		}
		out.State = "executing"
		out.ApprovedBy = &actor
		if e = t.Save(&out).Error; e != nil {
			return e
		}
		at, e := instant(t)
		if e != nil {
			return e
		}
		transfer := organization.ResponsibilityTransferInput{FromOperatorID: out.ChannelID, ToOperatorID: brand, TakeoverID: out.ID, ActorID: actor, Evidence: out.ContractReference + " | " + out.Evidence, Customers: snapshot.Customers, AccountIDs: map[string]string{}, HoldIDs: map[string]string{}}
		ids := []string{}
		for customer := range snapshot.Customers {
			ids = append(ids, customer)
		}
		sort.Strings(ids)
		for _, customer := range ids {
			m, e := s.Ledger.TransferCustomerAssets(ctx, t, ledger.CustomerTransfer{FromOperatorID: out.ChannelID, ToOperatorID: brand, TakeoverID: out.ID, CustomerID: customer, ActorID: actor, Evidence: transfer.Evidence, At: at})
			if e != nil {
				return e
			}
			for from, to := range m.AccountIDs {
				transfer.AccountIDs[from] = to
			}
			for from, to := range m.HoldIDs {
				transfer.HoldIDs[from] = to
			}
		}
		if e = s.Commerce.TransferResponsibility(ctx, t, transfer); e != nil {
			return fmt.Errorf("takeover commerce: %w", e)
		}
		if e = s.Incentives.TransferResponsibility(ctx, t, transfer); e != nil {
			return fmt.Errorf("takeover incentives: %w", e)
		}
		if e = s.Cooperation.TransferSalesCommitments(ctx, t, transfer); e != nil {
			return fmt.Errorf("takeover commitments: %w", e)
		}
		if _, e = s.Organization.ApplyChannelTakeoverOwnership(ctx, t, organization.TakeoverOwnershipInput{ChannelID: out.ChannelID, BrandOwnerOperatorID: brand, ExpectedVersions: snapshot.Customers, EvidenceReference: out.ContractReference}); e != nil {
			return fmt.Errorf("takeover ownership: %w", e)
		}
		// The entire procurement balance is retired in the closing statement. The
		// service offset may include gifts; only contract-refundable paid remainder
		// can offset cash. No part becomes the brand's own upstream inventory.
		if snapshot.Procurement.Quantity.Sign() > 0 {
			if _, e = s.Cooperation.RemoveProcurement(ctx, t, cooperation.RemovalInput{BuyerID: out.ChannelID, BusinessKey: "takeover:" + out.ID, ActorID: actor, Mode: "takeover", Reason: "one-time channel responsibility settlement", Evidence: out.ContractReference, Quantity: snapshot.Procurement.Quantity, At: at, Guard: takeoverRemovalGuard{s, out}}); e != nil {
				return fmt.Errorf("takeover procurement: %w", e)
			}
		}
		if e = s.postTakeoverReceivable(ctx, t, out, actor); e != nil {
			return e
		}
		if e = t.Table("organization_supply_agreements").Where("buyer_id=?", out.ChannelID).Update("status", "exited").Error; e != nil {
			return e
		}
		if e = t.Table("organization_operators").Where("id=?", out.ChannelID).Update("lifecycle", "exiting").Error; e != nil {
			return e
		}
		out.State = "completed"
		out.CompletedAt = &at
		if e = t.Save(&out).Error; e != nil {
			return e
		}
		return s.event(t, brand, out.ID, "takeover.completed", actor, transfer.Evidence, map[string]any{"takeover": out, "customers": ids, "account_mapping": transfer.AccountIDs, "hold_mapping": transfer.HoldIDs})
	})
	return out, e
}
func (s *Service) postTakeoverReceivable(ctx context.Context, t *gorm.DB, p Takeover, actor string) error {
	amount := p.SettlementDueMoney
	if amount.IsZero() {
		return nil
	}
	debtor, creditor := p.ChannelID, p.BrandOperatorID
	if amount.Sign() < 0 {
		debtor, creditor = creditor, debtor
		amount = amount.Neg()
	}
	for _, spec := range []struct{ op, purpose string }{{debtor, "counterparty:takeover_payable"}, {creditor, "counterparty:takeover_receivable"}} {
		a, e := s.account(ctx, t, spec.op, p.ID, spec.purpose)
		if e != nil {
			return e
		}
		control, e := s.account(ctx, t, spec.op, spec.op, "control:takeover_settlement")
		if e != nil {
			return e
		}
		if _, e = s.Ledger.Post(ctx, t, ledger.Posting{OperatorID: spec.op, BusinessKey: "takeover-settlement:" + p.ID, Kind: "takeover_counterparty_settlement", SourceID: p.ID, ActorID: actor, Reason: p.ContractReference, Lines: []ledger.Line{{AccountID: a.ID, Amount: amount}, {AccountID: control.ID, Amount: amount.Neg()}}}); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) Takeovers(ctx context.Context, actor, brand string) ([]Takeover, error) {
	if e := s.Organization.RequirePermission(ctx, nil, actor, brand, "cooperation.manage"); e != nil {
		return nil, e
	}
	var out []Takeover
	e := s.DB.WithContext(ctx).Where("brand_operator_id=?", brand).Order("created_at DESC,id").Limit(200).Find(&out).Error
	return out, e
}
func (s *Service) CancelTakeover(ctx context.Context, actor, brand, id, evidence string) (Takeover, error) {
	var out Takeover
	if evidence == "" {
		return out, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		var e error
		out, e = s.lockTakeover(ctx, t, actor, brand, id)
		if e != nil {
			return e
		}
		if out.State == "cancelled" {
			return nil
		}
		if out.State != "preparing" && out.State != "awaiting_approval" {
			return ErrState
		}
		// Do not silently restart model calls, sales or procurement when a takeover
		// is cancelled. Their explicit operational gates are re-opened by an owner.
		out.State = "cancelled"
		if e = t.Save(&out).Error; e != nil {
			return e
		}
		return s.event(t, brand, id, "takeover.cancelled", actor, evidence, out)
	})
	return out, e
}
func (s *Service) RecordTakeoverSettlement(ctx context.Context, actor, brand, id string, proof ExecutionProof) (Takeover, error) {
	var out Takeover
	if strings.TrimSpace(proof.Reference) == "" {
		proof.Reference = "manual:takeover:" + fingerprint([]string{id, proof.BusinessKey})
	}
	foreignAmount, parseErr := ledger.ParseDecimal(proof.Amount)
	if parseErr != nil || foreignAmount.Sign() <= 0 || !validAmount(proof.ActualMoney) || !proof.FeeMoney.Valid() || proof.FeeMoney.Sign() < 0 || proof.Currency == "" || proof.Currency != strings.ToUpper(proof.Currency) || proof.Reference == "" || proof.BusinessKey == "" || len(proof.BusinessKey) > 150 || proof.Evidence == "" || proof.OccurredAt.IsZero() {
		return out, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		monetary, e := ledger.GetMonetary(ctx, t)
		if e != nil {
			return e
		}
		if proof.Currency == monetary.Currency && foreignAmount.Cmp(proof.ActualMoney) != 0 {
			return ErrInvalid
		}
		out, e = s.lockTakeover(ctx, t, actor, brand, id)
		if e != nil {
			return e
		}
		if e = s.Organization.RequirePermission(ctx, t, actor, brand, "refunds.manage"); e != nil {
			return e
		}
		if out.State != "completed" {
			return ErrState
		}
		fp := fingerprint(proof)
		var old struct{ Fingerprint string }
		q := t.Table("remediation_takeover_settlements").Where("takeover_id=? AND business_key=?", id, proof.BusinessKey).Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if old.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		total := out.SettlementDueMoney
		debtor, creditor := out.ChannelID, brand
		if total.Sign() < 0 {
			total = total.Neg()
			debtor, creditor = creditor, debtor
		}
		if out.SettledMoney.Add(proof.ActualMoney).Cmp(total) > 0 {
			return ErrConflict
		}
		at, e := instant(t)
		if e != nil {
			return e
		}
		if proof.OccurredAt.After(at.Add(5 * time.Minute)) {
			return ErrInvalid
		}
		for _, spec := range []struct {
			op, purpose string
			cashSign    int
		}{{debtor, "counterparty:takeover_payable", 1}, {creditor, "counterparty:takeover_receivable", -1}} {
			a, e := s.account(ctx, t, spec.op, id, spec.purpose)
			if e != nil {
				return e
			}
			cash, e := s.account(ctx, t, spec.op, spec.op, "control:external_cash")
			if e != nil {
				return e
			}
			amount := proof.ActualMoney
			lines := []ledger.Line{{AccountID: a.ID, Amount: amount.Neg()}, {AccountID: cash.ID, Amount: amount}}
			if spec.cashSign < 0 {
				control, e := s.account(ctx, t, spec.op, spec.op, "control:takeover_settlement")
				if e != nil {
					return e
				}
				lines = []ledger.Line{{AccountID: a.ID, Amount: amount.Neg()}, {AccountID: cash.ID, Amount: amount.Neg()}, {AccountID: control.ID, Amount: amount.Add(amount)}}
			} else if proof.FeeMoney.Sign() > 0 {
				// The payer records its verified sending fee separately. The agreed
				// USD debt is discharged only by the amount actually received.
				fees, e := s.account(ctx, t, spec.op, spec.op, "control:merchant_fees")
				if e != nil {
					return e
				}
				lines[1].Amount = amount.Add(proof.FeeMoney)
				lines = append(lines, ledger.Line{AccountID: fees.ID, Amount: proof.FeeMoney.Neg()})
			}
			if _, e = s.Ledger.Post(ctx, t, ledger.Posting{OperatorID: spec.op, BusinessKey: "takeover-cash:" + id + ":" + fingerprint(proof.BusinessKey), Kind: "takeover_cash_settlement", SourceID: id, ActorID: actor, Reason: proof.Reference + " | " + proof.Evidence, Lines: lines}); e != nil {
				return e
			}
		}
		if e = t.Exec("INSERT INTO remediation_takeover_settlements(id,takeover_id,business_key,fingerprint,amount_money,fee_money,currency,original_amount,reference,evidence,occurred_at,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)", uuid.NewString(), id, proof.BusinessKey, fp, proof.ActualMoney, proof.FeeMoney, proof.Currency, proof.Amount, proof.Reference, proof.Evidence, proof.OccurredAt, at).Error; e != nil {
			return e
		}
		out.SettledMoney = out.SettledMoney.Add(proof.ActualMoney)
		if e = t.Save(&out).Error; e != nil {
			return e
		}
		return s.event(t, brand, id, "takeover.cash_settled", actor, proof.Evidence, proof)
	})
	return out, e
}
