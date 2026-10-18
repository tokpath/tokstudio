package cooperation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PurchaseQuote struct {
	Name                 string          `json:"name"`
	TermsSummary         string          `json:"terms_summary"`
	ContractAttachmentID string          `json:"contract_attachment_id,omitempty"`
	Version              int64           `json:"version"`
	ExitPolicy           string          `json:"exit_policy,omitempty" gorm:"-"`
	ID                   string          `json:"id"`
	SupplierID           string          `json:"supplier_id"`
	BuyerID              string          `json:"buyer_id"`
	PriceUSD             ledger.Decimal  `json:"price_usd"`
	PaidATG              ledger.Decimal  `json:"paid_atg"`
	GiftATG              ledger.Decimal  `json:"gift_atg"`
	ExitTerms            json.RawMessage `json:"exit_terms" gorm:"type:jsonb"`
	TermsReference       string          `json:"terms_reference"`
	ValidUntil           *time.Time      `json:"valid_until"`
	Enabled              bool            `json:"enabled"`
	CreatedBy            string          `json:"created_by"`
	CreatedAt            time.Time       `json:"created_at"`
}

func (PurchaseQuote) TableName() string { return "cooperation_purchase_quotes" }

type PurchaseOrder struct {
	Name                 string          `json:"name"`
	TermsSummary         string          `json:"terms_summary"`
	ContractAttachmentID string          `json:"contract_attachment_id,omitempty"`
	QuoteVersion         int64           `json:"quote_version"`
	ID                   string          `json:"id"`
	SupplierID           string          `json:"supplier_id"`
	BuyerID              string          `json:"buyer_id"`
	QuoteID              string          `json:"quote_id"`
	BusinessKey          string          `json:"business_key"`
	Fingerprint          string          `json:"-"`
	DueUSD               ledger.Decimal  `json:"due_usd"`
	AllocatedUSD         ledger.Decimal  `json:"allocated_usd"`
	PaidATG              ledger.Decimal  `json:"paid_atg"`
	GiftATG              ledger.Decimal  `json:"gift_atg"`
	ExitTerms            json.RawMessage `json:"exit_terms" gorm:"type:jsonb"`
	TermsReference       string          `json:"terms_reference"`
	Status               string          `json:"status"`
	ExpiresAt            time.Time       `json:"expires_at"`
	CreatedAt            time.Time       `json:"created_at"`
	CompletedAt          *time.Time      `json:"completed_at"`
}

func (PurchaseOrder) TableName() string { return "cooperation_purchase_orders" }

type PurchaseAllocation struct {
	ID, OrderID, ReceiptReservationID, ReceiptID, BusinessKey, Fingerprint string
	AmountUSD                                                              ledger.Decimal
	CreatedAt                                                              time.Time
}

func (PurchaseAllocation) TableName() string { return "cooperation_purchase_allocations" }

func (s *Service) PublishPurchaseQuote(ctx context.Context, tx *gorm.DB, actor string, in PurchaseQuote) (PurchaseQuote, error) {
	var out PurchaseQuote
	policy, err := quoteExitPolicy(in.ExitPolicy, in.ExitTerms)
	if err != nil {
		return out, err
	}
	if in.PriceUSD.Sign() <= 0 || in.PaidATG.Sign() <= 0 || in.GiftATG.Sign() < 0 || len(in.Name) > 200 || len(in.TermsSummary) > 10000 {
		return out, ledger.ErrAmount
	}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		agreement, e := s.Organization.GetSupplyAgreement(ctx, t, in.BuyerID)
		if e != nil {
			return e
		}
		if agreement.SupplierID != in.SupplierID {
			return organization.ErrInvalidRelationship
		}
		if e = s.Organization.RequirePermission(ctx, t, actor, in.SupplierID, "cooperation.manage"); e != nil {
			return e
		}
		now, e := businessTime(ctx, t)
		if e != nil {
			return e
		}
		if in.ValidUntil != nil && !in.ValidUntil.After(now) {
			return ErrConflict
		}
		in.ContractAttachmentID = strings.TrimSpace(in.ContractAttachmentID)
		if e = s.Organization.RequireSupplyContract(ctx, t, in.SupplierID, in.ContractAttachmentID); e != nil {
			return e
		}
		if e = t.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "quote-version:"+in.SupplierID+":"+in.BuyerID).Error; e != nil {
			return e
		}
		if e = t.Model(&PurchaseQuote{}).Where("supplier_id=? AND buyer_id=?", in.SupplierID, in.BuyerID).Select("COALESCE(MAX(version),0)+1").Scan(&in.Version).Error; e != nil {
			return e
		}
		in.ID = uuid.NewString()
		in.Name, in.TermsSummary = strings.TrimSpace(in.Name), strings.TrimSpace(in.TermsSummary)
		if in.Name == "" {
			in.Name = fmt.Sprintf("%s ATG / %s USD 供货报价", in.PaidATG.String(), in.PriceUSD.String())
		}
		in.TermsReference = fmt.Sprintf("procurement-quote:%s:v%d", in.ID, in.Version)
		in.ExitTerms, _ = json.Marshal(ProcurementExitTerms{Version: 1, VoluntaryRefund: policy == "remaining_paid_proportional", Method: policy, TermsReference: fmt.Sprintf("procurement-exit:%s:v%d", in.ID, in.Version)})
		in.ExitPolicy = policy
		in.CreatedBy = actor
		in.CreatedAt = now
		out = in
		return t.Create(&out).Error
	})
	return out, e
}
func (s *Service) SetQuoteEnabled(ctx context.Context, tx *gorm.DB, actor, id string, enabled bool) error {
	return s.within(ctx, tx, func(t *gorm.DB) error {
		var q PurchaseQuote
		if e := t.First(&q, "id=?", id).Error; e != nil {
			return e
		}
		if e := s.Organization.RequirePermission(ctx, t, actor, q.SupplierID, "cooperation.manage"); e != nil {
			return e
		}
		return t.Model(&PurchaseQuote{}).Where("id=?", id).Update("enabled", enabled).Error
	})
}
func (s *Service) CreatePurchaseOrder(ctx context.Context, tx *gorm.DB, actor, buyer, quoteID, key string) (PurchaseOrder, error) {
	var out PurchaseOrder
	if key == "" {
		return out, ErrConflict
	}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, buyer, "cooperation.manage"); e != nil {
			return e
		}
		fp := hash([]string{buyer, quoteID, key})
		if e := t.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "purchase:"+buyer+":"+key).Error; e != nil {
			return e
		}
		e := t.First(&out, "buyer_id=? AND business_key=?", buyer, key).Error
		if e == nil {
			if out.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		var q PurchaseQuote
		if e = t.First(&q, "id=? AND buyer_id=?", quoteID, buyer).Error; e != nil {
			return e
		}
		now, e := businessTime(ctx, t)
		if e != nil {
			return e
		}
		if !q.Enabled || (q.ValidUntil != nil && !now.Before(*q.ValidUntil)) {
			return ErrConflict
		}
		agreement, e := s.Organization.GetSupplyAgreement(ctx, t, buyer)
		if e != nil {
			return e
		}
		if agreement.Status != "active" || agreement.SupplierID != q.SupplierID {
			return organization.ErrBusinessClosed
		}
		snapshot, rates, e := s.coverage(ctx, t, q.SupplierID, now)
		if e != nil {
			return e
		}
		if e = s.Lock(ctx, t, buyer); e != nil {
			return e
		}
		qty := q.PaidATG.Add(q.GiftATG)
		if snapshot.HasUpstream {
			needed, e := conservative(qty, rates.Children[buyer])
			if e != nil {
				return e
			}
			if needed.Cmp(snapshot.Space) > 0 {
				return ErrCoverage
			}
		}
		out = PurchaseOrder{Name: q.Name, TermsSummary: q.TermsSummary, ContractAttachmentID: q.ContractAttachmentID, QuoteVersion: q.Version, ID: uuid.NewString(), SupplierID: q.SupplierID, BuyerID: buyer, QuoteID: q.ID, BusinessKey: key, Fingerprint: fp, DueUSD: q.PriceUSD, PaidATG: q.PaidATG, GiftATG: q.GiftATG, ExitTerms: q.ExitTerms, TermsReference: q.TermsReference, Status: "awaiting_payment", ExpiresAt: now.Add(30 * time.Minute), CreatedAt: now}
		if q.ValidUntil != nil && q.ValidUntil.Before(out.ExpiresAt) {
			out.ExpiresAt = *q.ValidUntil
		}
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		commitment := SalesCommitment{ID: uuid.NewString(), OperatorID: q.SupplierID, BuyerID: &out.BuyerID, OrderID: out.ID, ATG: qty, Status: "active", CatalogVersion: snapshot.CatalogVersion, CreatedAt: now, UpdatedAt: now}
		return t.Create(&commitment).Error
	})
	return out, e
}

// FundPurchase is called only after the collector has authenticated the receipt.
// The shared commerce receipt allocation prevents reuse in a retail order.
func (s *Service) FundPurchase(ctx context.Context, tx *gorm.DB, collector *commerce.Service, actor, orderID, receiptID, key string, amount ledger.Decimal) (PurchaseOrder, error) {
	return s.fundPurchase(ctx, tx, collector, actor, orderID, receiptID, key, amount, false)
}

// ApplyVerifiedPurchaseReceipt is a trusted adapter workflow. The collector's
// original verified receipt and timing/allocation checks remain mandatory.
func (s *Service) ApplyVerifiedPurchaseReceipt(ctx context.Context, tx *gorm.DB, collector *commerce.Service, orderID, receiptID, key string, amount ledger.Decimal) (PurchaseOrder, error) {
	return s.fundPurchase(ctx, tx, collector, "system:procurement-collection", orderID, receiptID, key, amount, true)
}
func (s *Service) fundPurchase(ctx context.Context, tx *gorm.DB, collector *commerce.Service, actor, orderID, receiptID, key string, amount ledger.Decimal, trusted bool) (PurchaseOrder, error) {
	var out PurchaseOrder
	if collector == nil || amount.Sign() <= 0 || key == "" {
		return out, ledger.ErrAmount
	}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&out, "id=?", orderID).Error; e != nil {
			return e
		}
		if !trusted {
			if e := s.Organization.RequirePermission(ctx, t, actor, out.SupplierID, "payments.manage"); e != nil {
				return e
			}
		}
		fp := hash([]string{orderID, receiptID, key, amount.String()})
		var previous PurchaseAllocation
		e := t.First(&previous, "order_id=? AND business_key=?", orderID, key).Error
		if e == nil {
			if previous.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if out.Status == "completed" || amount.Cmp(out.DueUSD.Sub(out.AllocatedUSD)) > 0 {
			return ErrConflict
		}
		reservation, e := collector.ReserveReceiptFunds(ctx, t, commerce.ReceiptFundsInput{OperatorID: out.SupplierID, ReceiptID: receiptID, DestinationID: out.ID, BusinessKey: "wholesale:" + out.ID + ":" + key, ActorID: actor, AmountUSD: amount, Deadline: out.ExpiresAt})
		if e != nil {
			return e
		}
		out.AllocatedUSD = out.AllocatedUSD.Add(amount)
		if out.AllocatedUSD.Cmp(out.DueUSD) == 0 {
			out.Status = "paid"
		}
		if e = t.Save(&out).Error; e != nil {
			return e
		}
		now, e := businessTime(ctx, t)
		if e != nil {
			return e
		}
		allocation := PurchaseAllocation{ID: uuid.NewString(), OrderID: out.ID, ReceiptReservationID: reservation.ID, ReceiptID: receiptID, BusinessKey: key, Fingerprint: fp, AmountUSD: amount, CreatedAt: now}
		return t.Create(&allocation).Error
	})
	return out, e
}
func (s *Service) FulfillPurchase(ctx context.Context, tx *gorm.DB, orderID, actor string) (ProcurementLot, error) {
	var lot ProcurementLot
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		var o PurchaseOrder
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&o, "id=?", orderID).Error; e != nil {
			return e
		}
		if o.Status == "completed" {
			return t.First(&lot, "order_id=?", o.ID).Error
		}
		if o.Status != "paid" || o.AllocatedUSD.Cmp(o.DueUSD) != 0 {
			return commerce.ErrNotPaid
		}
		if e := s.Lock(ctx, t, o.BuyerID); e != nil {
			return e
		}
		funds, e := s.Ledger.EnsureAccount(ctx, t, ledger.Account{OperatorID: o.SupplierID, OwnerID: o.ID, Purpose: "order:funds", Unit: "USD"})
		if e != nil {
			return e
		}
		sales, e := s.Ledger.EnsureAccount(ctx, t, ledger.Account{OperatorID: o.SupplierID, OwnerID: o.SupplierID, Purpose: "control:wholesale_sales", Unit: "USD", AllowNegative: true})
		if e != nil {
			return e
		}
		_, e = s.Ledger.Post(ctx, t, ledger.Posting{OperatorID: o.SupplierID, BusinessKey: "wholesale:" + o.ID, Kind: "wholesale_sale", SourceID: o.ID, ActorID: actor, Reason: "verified fully prepaid procurement delivery", Lines: []ledger.Line{{AccountID: funds.ID, Amount: o.DueUSD.Neg()}, {AccountID: sales.ID, Amount: o.DueUSD}}})
		if e != nil {
			return e
		}
		now, e := businessTime(ctx, t)
		if e != nil {
			return e
		}
		lot, e = s.DeliverPurchase(ctx, t, PurchaseInput{BuyerID: o.BuyerID, SupplierID: o.SupplierID, OrderID: o.ID, ReceiptReference: "purchase-order:" + o.ID, ActorID: actor, PaidUSD: o.DueUSD, PaidATG: o.PaidATG, GiftATG: o.GiftATG, ExitTerms: o.ExitTerms, At: now})
		if e != nil {
			return e
		}
		if e = t.Model(&SalesCommitment{}).Where("order_id=?", o.ID).Updates(map[string]any{"status": "fulfilled", "updated_at": now}).Error; e != nil {
			return e
		}
		return t.Model(&PurchaseOrder{}).Where("id=?", o.ID).Updates(map[string]any{"status": "completed", "completed_at": now}).Error
	})
	return lot, e
}
func (s *Service) ClosePurchase(ctx context.Context, tx *gorm.DB, actor, id string, proof commerce.PaymentClosureProof) error {
	if !proof.VerifiedUnpaid || proof.EvidenceReference == "" {
		return commerce.ErrPaymentUnknown
	}
	return s.within(ctx, tx, func(t *gorm.DB) error {
		var o PurchaseOrder
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&o, "id=?", id).Error; e != nil {
			return e
		}
		if e := s.Organization.RequirePermission(ctx, t, actor, o.BuyerID, "cooperation.manage"); e != nil {
			return e
		}
		if o.Status == "closed" {
			return nil
		}
		if o.Status != "awaiting_payment" || o.AllocatedUSD.Sign() > 0 {
			return commerce.ErrOrderState
		}
		if e := s.Lock(ctx, t, o.BuyerID); e != nil {
			return e
		}
		if e := t.Model(&PurchaseOrder{}).Where("id=?", id).Update("status", "closed").Error; e != nil {
			return e
		}
		return t.Model(&SalesCommitment{}).Where("order_id=?", id).Updates(map[string]any{"status": "released", "updated_at": gorm.Expr("clock_timestamp()")}).Error
	})
}

// quoteExitPolicy translates the business choice into the sole accepted rule.
// Schema versions and references are generated by the service, never by a form.
func quoteExitPolicy(policy string, raw json.RawMessage) (string, error) {
	policy = strings.TrimSpace(policy)
	if len(raw) > 0 && string(raw) != "null" {
		var legacy ProcurementExitTerms
		if json.Unmarshal(raw, &legacy) != nil || (legacy.Version != 0 && legacy.Version != 1) || (legacy.Method != "remaining_paid_proportional" && legacy.Method != "non_refundable") || (legacy.VoluntaryRefund && legacy.Method != "remaining_paid_proportional") {
			return "", ErrExitTerms
		}
		legacyPolicy := "non_refundable"
		if legacy.VoluntaryRefund {
			legacyPolicy = "remaining_paid_proportional"
		}
		if policy != "" && policy != legacyPolicy {
			return "", ErrExitTerms
		}
		policy = legacyPolicy
	}
	if policy == "" {
		policy = "non_refundable"
	}
	if policy != "non_refundable" && policy != "remaining_paid_proportional" {
		return "", ErrExitTerms
	}
	return policy, nil
}

func ProcurementExitPolicySummary(policy string) string {
	if policy == "remaining_paid_proportional" {
		return "主动退出并清算存量责任后，剩余购买额度按原实付比例退回；赠送额度不退。须另行办理退出及实际退款。"
	}
	return "主动退出时剩余采购余额不退；须先清算存量责任。供货方不能履约时另行办理异常退款。"
}

func ProcurementExitTermsSummary(raw json.RawMessage) string {
	t, err := ParseProcurementExitTerms(raw)
	if err != nil {
		return "原成交退出规则待核实"
	}
	policy := "non_refundable"
	if t.VoluntaryRefund {
		policy = t.Method
	}
	return ProcurementExitPolicySummary(policy)
}
