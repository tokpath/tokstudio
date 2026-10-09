package commission

import "context"

// PersonalSummary is computed over the complete personal history, never a list page.
type PersonalSummary struct {
	EarnedMinor    int64 `json:"earned_minor"`
	FrozenMinor    int64 `json:"frozen_minor"`
	AvailableMinor int64 `json:"available_minor"`
	HeldMinor      int64 `json:"held_minor"`
	SettledMinor   int64 `json:"settled_minor"`
	PaidMinor      int64 `json:"paid_minor"`
	ReversedMinor  int64 `json:"reversed_minor"`
}

type PersonalPage struct {
	Summary          PersonalSummary  `json:"summary"`
	Entries          []EntryView      `json:"entries"`
	Settlements      []SettlementView `json:"settlements"`
	EntriesTotal     int64            `json:"entries_total"`
	SettlementsTotal int64            `json:"settlements_total"`
	Page             int              `json:"page"`
	PageSize         int              `json:"page_size"`
}

// Empty membership must mean no data; the legacy list APIs use empty as unrestricted.
func (s *Service) PersonalPage(ctx context.Context, roleIDs []string, page int) (*PersonalPage, error) {
	if page < 1 {
		page = 1
	}
	if page > 100000 {
		page = 100000
	}
	const size = 25
	out := &PersonalPage{Entries: []EntryView{}, Settlements: []SettlementView{}, Page: page, PageSize: size}
	if len(roleIDs) == 0 {
		return out, nil
	}
	q := s.db.WithContext(ctx).Model(&entryRow{}).Where("beneficiary_role_id IN ?", roleIDs)
	if err := q.Select(`COALESCE(SUM(amount_minor), 0) AS earned_minor,
	COALESCE(SUM(CASE WHEN status = 'frozen' THEN amount_minor ELSE 0 END), 0) AS frozen_minor,
	COALESCE(SUM(CASE WHEN status = 'available' THEN amount_minor ELSE 0 END), 0) AS available_minor,
	COALESCE(SUM(CASE WHEN status = 'held' THEN amount_minor ELSE 0 END), 0) AS held_minor,
	COALESCE(SUM(CASE WHEN status = 'settled' THEN amount_minor ELSE 0 END), 0) AS settled_minor,
	COALESCE(SUM(CASE WHEN status = 'paid' THEN amount_minor ELSE 0 END), 0) AS paid_minor,
	COALESCE(SUM(CASE WHEN reversal_of IS NOT NULL THEN amount_minor ELSE 0 END), 0) AS reversed_minor`).Scan(&out.Summary).Error; err != nil {
		return nil, err
	}
	if err := q.Count(&out.EntriesTotal).Error; err != nil {
		return nil, err
	}
	var rows []entryRow
	if err := s.db.WithContext(ctx).Where("beneficiary_role_id IN ?", roleIDs).Order("created_at DESC, id DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out.Entries = append(out.Entries, *entryView(row))
	}
	if err := s.db.WithContext(ctx).Model(&settleRow{}).Where("beneficiary_role_id IN ?", roleIDs).Count(&out.SettlementsTotal).Error; err != nil {
		return nil, err
	}
	// Reuse payout and recovery facts, scoped to the settlements on this page.
	var settlements []settleRow
	if err := s.db.WithContext(ctx).Where("beneficiary_role_id IN ?", roleIDs).Order("created_at DESC, id DESC").Offset((page - 1) * size).Limit(size).Find(&settlements).Error; err != nil {
		return nil, err
	}
	views, err := s.settlementViews(ctx, settlements)
	if err != nil {
		return nil, err
	}
	out.Settlements = views
	return out, nil
}
