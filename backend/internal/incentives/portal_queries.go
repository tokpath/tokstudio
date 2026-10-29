package incentives

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type CommissionView struct {
	ID            string         `json:"id"`
	SourceID      string         `json:"source_id"`
	OrderID       string         `json:"order_id"`
	Type          string         `json:"type"`
	Hop           int            `json:"hop"`
	Status        string         `json:"status"`
	AmountMoney   ledger.Decimal `json:"amount_money"`
	BasisMoney    ledger.Decimal `json:"basis_money"`
	ReversedMoney ledger.Decimal `json:"reversed_money"`
	OffsetMoney   ledger.Decimal `json:"offset_money"`
	CreatedAt     time.Time      `json:"created_at"`
	ConfirmsAt    time.Time      `json:"confirms_at"`
}

func pageLimit(limit int) int {
	if limit < 1 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}
func incentiveCursor(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano) + "|" + id))
}
func readIncentiveCursor(cursor string) (time.Time, string, error) {
	b, e := base64.RawURLEncoding.DecodeString(cursor)
	if e != nil {
		return time.Time{}, "", ErrInvalidInput
	}
	parts := strings.Split(string(b), "|")
	if len(parts) != 2 || !validID(parts[1]) {
		return time.Time{}, "", ErrInvalidInput
	}
	at, e := time.Parse(time.RFC3339Nano, parts[0])
	if e != nil {
		return time.Time{}, "", ErrInvalidInput
	}
	return at, parts[1], nil
}

// CommissionViews uses the exact eligible base persisted for this event/hop.
// It never reconstructs the base from today's commission rates.
func (s *Service) CommissionViews(ctx context.Context, tx *gorm.DB, operator, customer, cursor, query string, limit int) (out []CommissionView, next string, err error) {
	own, e := s.org.CurrentOwnership(ctx, tx, customer)
	if e != nil {
		return nil, "", e
	}
	if own.OperatorID != operator {
		return nil, "", ErrNotEligible
	}
	q := s.dbFor(ctx, tx).Table("incentives_commission_entries c").Select("c.id,e.source_event_id AS source_id,c.order_id,e.kind AS type,c.hop,c.status,c.amount_money,b.base_money AS basis_money,c.reversed_money,c.offset_money,c.created_at,c.confirm_at AS confirms_at").Joins("JOIN incentives_confirmation_events e ON e.id=c.event_id").Joins("JOIN incentives_commission_bases b ON b.event_id=c.event_id AND b.hop=c.hop AND b.beneficiary_id=c.beneficiary_id").Where("c.operator_id=? AND c.beneficiary_id=?", operator, customer)
	if term := strings.TrimSpace(query); term != "" {
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(term) + "%"
		q = q.Where(`(c.id::text ILIKE ? ESCAPE '\' OR e.source_event_id::text ILIKE ? ESCAPE '\' OR COALESCE(c.order_id::text,'') ILIKE ? ESCAPE '\' OR e.kind ILIKE ? ESCAPE '\' OR c.status ILIKE ? ESCAPE '\')`, like, like, like, like, like)
	}
	if cursor != "" {
		at, id, e := readIncentiveCursor(cursor)
		if e != nil {
			return nil, "", e
		}
		q = q.Where("(c.created_at,c.id)<(?,?)", at, id)
	}
	limit = pageLimit(limit)
	if err = q.Order("c.created_at DESC,c.id DESC").Limit(limit + 1).Scan(&out).Error; err != nil {
		return
	}
	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1]
		next = incentiveCursor(last.CreatedAt, last.ID)
	}
	if out == nil {
		out = []CommissionView{}
	}
	return
}
func (s *Service) CurrentWithdrawalPolicy(ctx context.Context, tx *gorm.DB, operator string) (WithdrawalPolicy, error) {
	if !validID(operator) {
		return WithdrawalPolicy{}, ErrInvalidInput
	}
	var r withdrawalPolicyRow
	q := s.dbFor(ctx, tx).Where("operator_id=?", operator).Order("version DESC").Limit(1).Find(&r)
	if q.Error != nil {
		return WithdrawalPolicy{}, q.Error
	}
	if q.RowsAffected == 0 {
		return WithdrawalPolicy{}, ErrNotReady
	}
	return WithdrawalPolicy(r), nil
}

func (s *Service) WithdrawalsForOperator(ctx context.Context, tx *gorm.DB, actor, operator, cursor string, limit int, filters ...string) (out []Withdrawal, next string, err error) {
	if err = s.org.RequirePermission(ctx, tx, actor, operator, "withdrawals.manage"); err != nil {
		return
	}
	q := s.dbFor(ctx, tx).Model(&withdrawalRow{}).Where("operator_id=?", operator)
	if len(filters) > 0 && strings.TrimSpace(filters[0]) != "" {
		term := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(strings.TrimSpace(filters[0])) + "%"
		q = q.Where("(id::text ILIKE ? OR customer_id::text ILIKE ? OR recipient_reference ILIKE ?)", term, term, term)
	}
	if len(filters) > 1 && strings.TrimSpace(filters[1]) != "" {
		q = q.Where("status=?", strings.TrimSpace(filters[1]))
	}

	if cursor != "" {
		at, id, e := readIncentiveCursor(cursor)
		if e != nil {
			return nil, "", e
		}
		q = q.Where("(created_at,id)<(?,?)", at, id)
	}
	limit = pageLimit(limit)
	var rows []withdrawalRow
	if err = q.Order("created_at DESC,id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return
	}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = incentiveCursor(last.CreatedAt, last.ID)
	}
	out = make([]Withdrawal, len(rows))
	for i, r := range rows {
		out[i] = Withdrawal(r)
	}
	return
}
