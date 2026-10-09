package billing

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/platform/pagecursor"
)

type WalletLedgerPage struct {
	Items      []LedgerView `json:"items"`
	Total      int64        `json:"total"`
	NextCursor string       `json:"next_cursor"`
}

func (s *Service) PersonalLedger(ctx context.Context, userID, cursor string) (*WalletLedgerPage, error) {
	pos, err := pagecursor.Decode(cursor, userID, "ledger")
	if err != nil {
		return nil, err
	}
	out := &WalletLedgerPage{Items: []LedgerView{}}
	q := s.db.WithContext(ctx).Table("billing_ledger l").Joins("JOIN billing_wallets w ON w.id=l.wallet_id").Where("w.user_id=?", userID)
	if err := q.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	if pos != nil {
		q = q.Where("(l.created_at,l.id)<(?,?)", pos.At, pos.ID)
	}
	var rows []ledgerRow
	if err := q.Select("l.*").Order("l.created_at DESC,l.id DESC").Limit(26).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > 25 {
		rows = rows[:25]
		last := rows[24]
		out.NextCursor = pagecursor.Encode(last.CreatedAt, last.ID, userID, "ledger")
	}
	for _, r := range rows {
		out.Items = append(out.Items, LedgerView{ID: r.ID, EventType: r.EventType, AmountMinor: r.AmountMinor, BalanceAfter: r.BalanceAfterMinor, ReservedAfter: r.ReservedAfterMinor, ReferenceType: r.ReferenceType, ReferenceID: r.ReferenceID, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

type PersonalRecoveryPage struct {
	Items      []CommissionRecoveryView `json:"items"`
	Total      int64                    `json:"total"`
	NextCursor string                   `json:"next_cursor"`
}

func (s *Service) PersonalRecoveries(ctx context.Context, userID, cursor string) (*PersonalRecoveryPage, error) {
	pos, err := pagecursor.Decode(cursor, userID, "recoveries")
	if err != nil {
		return nil, err
	}
	out := &PersonalRecoveryPage{Items: []CommissionRecoveryView{}}
	q := s.db.WithContext(ctx).Table("billing_commission_recoveries r").Joins("JOIN billing_wallets w ON w.id=r.wallet_id").Where("w.user_id=?", userID)
	if err := q.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	if pos != nil {
		q = q.Where("(r.created_at,r.id)<(?,?)", pos.At, pos.ID)
	}
	if err := q.Select("r.id,r.settlement_id,r.commission_entry_id,r.amount_minor,r.recovered_minor,r.status,r.created_at").Order("r.created_at DESC,r.id DESC").Limit(26).Scan(&out.Items).Error; err != nil {
		return nil, err
	}
	if len(out.Items) > 25 {
		out.Items = out.Items[:25]
		last := out.Items[24]
		out.NextCursor = pagecursor.Encode(last.CreatedAt, last.ID, userID, "recoveries")
	}
	ids := []string{}
	indices := map[string]int{}
	for i := range out.Items {
		out.Items[i].Receipts = []RecoveryReceipt{}
		ids = append(ids, out.Items[i].ID)
		indices[out.Items[i].ID] = i
	}
	if len(ids) > 0 {
		var receipts []RecoveryReceipt
		if err := s.db.WithContext(ctx).Where("recovery_id IN ?", ids).Order("created_at,id").Find(&receipts).Error; err != nil {
			return nil, err
		}
		for _, r := range receipts {
			r.ActorUserID = ""
			r.ActorEmail = ""
			r.Note = ""
			i := indices[r.RecoveryID]
			out.Items[i].Receipts = append(out.Items[i].Receipts, r)
		}
	}
	return out, nil
}
