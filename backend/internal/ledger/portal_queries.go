package ledger

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// These read models require an authenticated application scope. Every query is
// constrained to an exact operator and, for customer views, an exact owner.
type CustomerAssetSummary struct {
	AvailableATG, HeldATG, PurchasedRemainingATG, GiftRemainingATG, ExpiredATG Decimal
}

func validPortalScope(operator, owner string) bool {
	if _, err := uuid.Parse(operator); err != nil {
		return false
	}
	if owner != "" {
		if _, err := uuid.Parse(owner); err != nil {
			return false
		}
	}
	return true
}

func (s *Service) CustomerAssetSummary(ctx context.Context, tx *gorm.DB, operator, customer string, at time.Time) (CustomerAssetSummary, error) {
	var result CustomerAssetSummary
	if !validPortalScope(operator, customer) || customer == "" {
		return result, ErrScope
	}
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		// Delivery and invocation use the database clock. A live balance must
		// use the same clock, even when an API host is a little ahead/behind.
		// An explicit time remains available for historical/domain queries.
		if at.IsZero() {
			if err := t.Raw("SELECT clock_timestamp()").Row().Scan(&at); err != nil {
				return err
			}
		}

		if err := t.Raw(`SELECT
			COALESCE(sum(CASE WHEN g.effective_at<=? AND (g.expires_at IS NULL OR g.expires_at>?) THEN a.balance-a.held ELSE 0 END),0)::text AS available_atg,
			COALESCE(sum(a.held),0)::text AS held_atg,
			COALESCE(sum(g.expired),0)::text AS expired_atg
			FROM ledger_grants g JOIN ledger_accounts a ON a.id=g.account_id
			WHERE g.operator_id=? AND g.customer_id=?`, at, at, operator, customer).Scan(&result).Error; err != nil {
			return err
		}
		var sources struct{ PurchasedRemainingATG, GiftRemainingATG Decimal }
		err := t.Raw(`SELECT
 COALESCE(sum(CASE WHEN src.kind='paid' THEN src.quantity-src.consumed-src.expired-src.cancelled ELSE 0 END),0)::text AS purchased_remaining_atg,
			COALESCE(sum(CASE WHEN src.kind<>'paid' THEN src.quantity-src.consumed-src.expired-src.cancelled ELSE 0 END),0)::text AS gift_remaining_atg
			FROM ledger_grant_sources src JOIN ledger_grants g ON g.id=src.grant_id
			WHERE g.operator_id=? AND g.customer_id=? AND g.effective_at<=?
			AND (g.expires_at IS NULL OR g.expires_at>?)`, operator, customer, at, at).Scan(&sources).Error
		result.PurchasedRemainingATG = sources.PurchasedRemainingATG
		result.GiftRemainingATG = sources.GiftRemainingATG
		return err
	})
	return result, err
}

// AccountGrants returns timing and source information for the selected page
// of accounts, along with the database time used to evaluate availability.
func (s *Service) AccountGrants(ctx context.Context, tx *gorm.DB, operator string, accounts []string) (map[string]Grant, time.Time, error) {
	out := make(map[string]Grant)
	if len(accounts) == 0 {
		return out, time.Time{}, nil
	}
	db := s.db.WithContext(ctx)
	if tx != nil {
		db = tx.WithContext(ctx)
	}
	var now time.Time
	if err := db.Raw("SELECT clock_timestamp()").Row().Scan(&now); err != nil {
		return nil, now, err
	}
	var grants []Grant
	if err := db.Where("operator_id=? AND account_id IN ?", operator, accounts).Find(&grants).Error; err != nil {
		return nil, now, err
	}
	for _, g := range grants {
		out[g.AccountID] = g
	}
	return out, now, nil
}

func (s *Service) FindAccount(ctx context.Context, tx *gorm.DB, operator, owner, purpose string) (Account, error) {
	var result Account
	if !validPortalScope(operator, owner) || owner == "" || purpose == "" {
		return result, ErrScope
	}
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Where("operator_id=? AND owner_id=? AND purpose=?", operator, owner, purpose).Take(&result).Error
	})
	return result, err
}
func (s *Service) ListOperatorAccounts(ctx context.Context, tx *gorm.DB, operator, owner, cursor, query string, limit int, statuses ...string) ([]Account, error) {
	return s.listOperatorAccounts(ctx, tx, operator, owner, cursor, query, limit, false, false, statuses...)
}

var businessAccountPurposes = []string{"customer:money", "commission:available", "commission:frozen", "procurement:atg"}

const customerGrantAccountPrefix = "customer:atg:"

// Business accounts hold customer assets or the operator's stock.
// Control, receipt and order accounts remain available in the complete ledger.
func IsBusinessAccount(purpose string) bool {
	return slices.Contains(businessAccountPurposes, purpose) || strings.HasPrefix(purpose, customerGrantAccountPrefix)
}

// BusinessAccountScope applies the same classification to a ledger account query.
func BusinessAccountScope(q *gorm.DB) *gorm.DB {
	return q.Where("(purpose IN ? OR purpose LIKE ?)", businessAccountPurposes, customerGrantAccountPrefix+"%")
}

func (s *Service) ListAssetAccounts(ctx context.Context, operator, cursor, query string, limit int, includeAuxiliary bool) ([]Account, error) {
	return s.listOperatorAccounts(ctx, nil, operator, "", cursor, query, limit, !includeAuxiliary, includeAuxiliary)
}

func (s *Service) listOperatorAccounts(ctx context.Context, tx *gorm.DB, operator, owner, cursor, query string, limit int, businessOnly, searchBusinessOwners bool, statuses ...string) ([]Account, error) {
	if !validPortalScope(operator, owner) || limit < 1 || limit > 100 {
		return nil, ErrScope
	}
	if cursor != "" {
		if _, err := uuid.Parse(cursor); err != nil {
			return nil, ErrScope
		}
	}
	var result []Account
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		q := t.Where("operator_id=?", operator).Order("id").Limit(limit)
		if businessOnly {
			q = BusinessAccountScope(q)
		}
		if len(statuses) > 0 && statuses[0] != "" && statuses[0] != "open" {
			q = q.Where("1=0")
		}
		if owner != "" {
			q = q.Where("owner_id=?", owner)
		}
		if cursor != "" {
			q = q.Where("id>?", cursor)
		}
		if query != "" {
			term := likePattern(query)
			clauses := []string{
				`id::text ILIKE ? ESCAPE '\'`,
				`owner_id::text ILIKE ? ESCAPE '\'`,
				`purpose ILIKE ? ESCAPE '\'`,
				`unit ILIKE ? ESCAPE '\'`,
				`EXISTS (SELECT 1 FROM organization_customers c LEFT JOIN organization_customer_identities i ON i.customer_id=c.id AND i.is_current=true WHERE c.id=ledger_accounts.owner_id AND (c.display_name ILIKE ? ESCAPE '\' OR i.email_normalized ILIKE ? ESCAPE '\'))`,
				`EXISTS (SELECT 1 FROM organization_operators o WHERE o.id=ledger_accounts.owner_id AND (o.name ILIKE ? ESCAPE '\' OR COALESCE(o.legal_name,'') ILIKE ? ESCAPE '\'))`,
			}
			args := []any{term, term, term, term, term, term, term, term}
			if searchBusinessOwners {
				// Auxiliary accounts belong to a business record. Search by the
				// same customer or buyer shown by that record, before pagination.
				clauses = append(clauses,
					`EXISTS (SELECT 1 FROM commerce_orders o JOIN organization_customers c ON c.id=o.customer_id LEFT JOIN organization_customer_identities i ON i.customer_id=c.id AND i.is_current=true WHERE o.operator_id=ledger_accounts.operator_id AND o.id=ledger_accounts.owner_id AND (c.display_name ILIKE ? ESCAPE '\' OR i.email_normalized ILIKE ? ESCAPE '\'))`,
					`EXISTS (SELECT 1 FROM commerce_receipts r JOIN commerce_receipt_allocations al ON al.receipt_id=r.id JOIN commerce_orders o ON o.id=al.order_id AND o.operator_id=r.operator_id JOIN organization_customers c ON c.id=o.customer_id LEFT JOIN organization_customer_identities i ON i.customer_id=c.id AND i.is_current=true WHERE r.operator_id=ledger_accounts.operator_id AND r.id=ledger_accounts.owner_id AND (c.display_name ILIKE ? ESCAPE '\' OR i.email_normalized ILIKE ? ESCAPE '\'))`,
					`EXISTS (SELECT 1 FROM cooperation_purchase_orders p JOIN organization_operators o ON o.id=p.buyer_id WHERE (p.supplier_id=ledger_accounts.operator_id OR p.buyer_id=ledger_accounts.operator_id) AND p.id=ledger_accounts.owner_id AND (o.name ILIKE ? ESCAPE '\' OR COALESCE(o.legal_name,'') ILIKE ? ESCAPE '\'))`,
					`EXISTS (SELECT 1 FROM commerce_receipts r JOIN cooperation_purchase_allocations al ON al.receipt_id=r.id JOIN cooperation_purchase_orders p ON p.id=al.order_id AND p.supplier_id=r.operator_id JOIN organization_operators o ON o.id=p.buyer_id WHERE r.operator_id=ledger_accounts.operator_id AND r.id=ledger_accounts.owner_id AND (o.name ILIKE ? ESCAPE '\' OR COALESCE(o.legal_name,'') ILIKE ? ESCAPE '\'))`,
				)
				for i := 0; i < 8; i++ {
					args = append(args, term)
				}
			}
			if purposes := accountPurposesMatching(query); len(purposes) > 0 {
				var parts []string
				for _, purpose := range purposes {
					parts = append(parts, "(purpose = ? OR purpose LIKE ? ESCAPE '\\')")
					args = append(args, purpose, likePattern(purpose+":"))
				}
				clauses = append(clauses, "("+strings.Join(parts, " OR ")+")")
			}
			if currency, err := currentSettlementCurrency(t); err == nil && strings.EqualFold(strings.TrimSpace(query), currency) {
				clauses = append(clauses, "unit = 'MONEY'")
			}
			q = q.Where(strings.Join(clauses, " OR "), args...)
		}
		return q.Find(&result).Error
	})
	return result, err
}

func likePattern(query string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(query))
	return "%" + escaped + "%"
}

func currentSettlementCurrency(tx *gorm.DB) (string, error) {
	var currency string
	err := tx.Raw(`SELECT currency FROM ledger_monetary_settings ORDER BY effective_at DESC LIMIT 1`).Row().Scan(&currency)
	return currency, err
}

// accountPurposesMatching maps the words shown on the account list back to the
// stored purpose. The page title "资产账户" is not one of those words.
func accountPurposesMatching(query string) []string {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return nil
	}
	labels := map[string]string{
		"支付余额":                 "customer:money",
		"payment balance":      "customer:money",
		"支払残高":                 "customer:money",
		"платёжный баланс":     "customer:money",
		"可用佣金":                 "commission:available",
		"available commission": "commission:available",
		"利用可能な紹介報酬":            "commission:available",
		"доступное вознаграждение": "commission:available",
		"冻结佣金":              "commission:frozen",
		"frozen commission": "commission:frozen",
		"凍結中の紹介報酬":          "commission:frozen",
		"замороженное вознаграждение": "commission:frozen",
		"采购额度":                     "procurement:atg",
		"procurement credits":      "procurement:atg",
		"仕入利用枠":                    "procurement:atg",
		"закупочная квота":         "procurement:atg",
		"订单已分配款":                   "order:funds",
		"funds allocated to order": "order:funds",
		"注文への割当済資金":                "order:funds",
		"средства, распределённые на заказ": "order:funds",
		"未分配收款":               "receipt:unallocated",
		"unallocated receipt": "receipt:unallocated",
		"未割当の入金":              "receipt:unallocated",
		"нераспределённое поступление": "receipt:unallocated",
		"通道未结应收":                        "control:provider_receivable",
		"unsettled channel receivables": "control:provider_receivable",
		"チャネル未決済債権":                     "control:provider_receivable",
		"непогашенная дебиторская задолженность канала": "control:provider_receivable",
		"外部现金往来":                    "control:external_cash",
		"external cash transfers":   "control:external_cash",
		"外部現金取引":                    "control:external_cash",
		"внешние денежные переводы": "control:external_cash",
		"经营方收款手续费":                  "control:merchant_fees",
		"operator receipt fees":     "control:merchant_fees",
		"運営者負担の入金手数料":               "control:merchant_fees",
		"комиссии оператора за приём платежей": "control:merchant_fees",
		"经营方退款手续费":                           "control:refund_fees",
		"operator refund fees":               "control:refund_fees",
		"運営者負担の返金手数料":                        "control:refund_fees",
		"комиссии оператора за возвраты":     "control:refund_fees",
		"套餐销售记账":                             "control:sales",
		"package sales account":              "control:sales",
		"プラン売上口座":                            "control:sales",
		"счёт продаж пакетов":                "control:sales",
		"供货销售记账":                             "control:wholesale_sales",
		"wholesale sales account":            "control:wholesale_sales",
		"卸売売上口座":                             "control:wholesale_sales",
		"счёт оптовых продаж":                "control:wholesale_sales",
		"客户权益平衡":                             "control:credits",
		"customer credit balancing":          "control:credits",
		"顧客利用枠の対照口座":                         "control:credits",
		"балансирующий счёт клиентских квот": "control:credits",
		"权益收回记账":                             "control:cancelled",
		"credit withdrawal account":          "control:cancelled",
		"利用枠回収の記帳口座":                         "control:cancelled",
		"счёт отзыва квот":                   "control:cancelled",
		"佣金计提记账":                             "control:commissions",
		"commission accrual account":         "control:commissions",
		"コミッション計上口座":                         "control:commissions",
		"счёт начисления комиссий":           "control:commissions",
		"佣金付款记账":                             "control:commission_payouts",
		"commission payout":                  "control:commission_payouts",
		"コミッション支払の記帳":                        "control:commission_payouts",
		"учёт выплаты комиссии":              "control:commission_payouts",
		"提现手续费":                              "control:withdrawal_fees",
		"withdrawal fee":                     "control:withdrawal_fees",
		"出金手数料":                              "control:withdrawal_fees",
		"комиссия за вывод":                  "control:withdrawal_fees",
		"采购额度平衡":                             "control:procurement",
		"procurement credit balancing":       "control:procurement",
		"仕入利用枠の対照口座":                         "control:procurement",
		"балансирующий счёт закупочной квоты": "control:procurement",
		"采购退出额度记账":                              "control:procurement_removal",
		"procurement exit credit account":       "control:procurement_removal",
		"仕入契約終了の利用枠記帳":                          "control:procurement_removal",
		"счёт квот при прекращении закупок":     "control:procurement_removal",
		"接管权益对账":                                "control:takeover",
		"takeover credit control":               "control:takeover",
		"引継ぎ利用権の照合":                             "control:takeover",
		"контроль переданных единиц":            "control:takeover",
		"接管结清对账":                                "control:takeover_settlement",
		"takeover settlement control":           "control:takeover_settlement",
		"引継ぎ精算の照合":                              "control:takeover_settlement",
		"контроль расчётов по передаче":         "control:takeover_settlement",
		"采购退出应付款":                               "procurement_exit:payable",
		"procurement exit payable":              "procurement_exit:payable",
		"仕入契約終了の未払金":                            "procurement_exit:payable",
		"к выплате при прекращении закупок":     "procurement_exit:payable",
		"采购退出应收款":                               "procurement_exit:receivable",
		"procurement exit receivable":           "procurement_exit:receivable",
		"仕入契約終了の未収金":                            "procurement_exit:receivable",
		"к получению при прекращении закупок":   "procurement_exit:receivable",
		"采购退出结算记账":                              "control:procurement_exit_obligation",
		"procurement exit settlement":           "control:procurement_exit_obligation",
		"仕入契約終了の精算記帳":                           "control:procurement_exit_obligation",
		"учёт расчётов при прекращении закупок": "control:procurement_exit_obligation",
		"外部收款往来":                                "control:external_receipt",
		"external receipts clearing":            "control:external_receipt",
		"外部入金の精算口座":                             "control:external_receipt",
		"расчёты по внешним поступлениям":       "control:external_receipt",
		"采购退款差额记账":                              "control:procurement_refund_excess",
		"procurement refund difference":         "control:procurement_refund_excess",
		"仕入返金の差額記帳":                             "control:procurement_refund_excess",
		"учёт разницы при возврате закупки":     "control:procurement_refund_excess",
		"客户 atg 权益":                             "customer:atg",
		"customer atg credits":                  "customer:atg",
		"顧客 atg 利用枠":                            "customer:atg",
		"единицы atg клиента":                   "customer:atg",
		"客户 atg":                                "customer:atg",
		"现金余额":                                  "customer:money",
	}
	var purposes []string
	seen := map[string]bool{}
	for label, purpose := range labels {
		if strings.Contains(label, needle) || strings.Contains(needle, label) {
			if !seen[purpose] {
				seen[purpose] = true
				purposes = append(purposes, purpose)
			}
		}
	}
	return purposes
}

func (s *Service) ListJournals(ctx context.Context, tx *gorm.DB, operator, owner, cursor string, limit int) ([]Journal, error) {
	if !validPortalScope(operator, owner) || limit < 1 || limit > 100 {
		return nil, ErrScope
	}
	if cursor != "" {
		if _, err := uuid.Parse(cursor); err != nil {
			return nil, ErrScope
		}
	}
	var result []Journal
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		q := t.Where("operator_id=?", operator).Order("created_at DESC, id DESC").Limit(limit)
		if cursor != "" {
			var anchor Journal
			if err := t.Where("operator_id=? AND id=?", operator, cursor).Take(&anchor).Error; err != nil {
				return ErrScope
			}
			q = q.Where("(created_at,id) < (?,?)", anchor.CreatedAt, anchor.ID)
		}
		if owner != "" {
			q = q.Where(`EXISTS (SELECT 1 FROM ledger_entries e JOIN ledger_accounts a ON a.id=e.account_id WHERE e.journal_id=ledger_journals.id AND a.operator_id=? AND a.owner_id=?)`, operator, owner)
		}
		return q.Find(&result).Error
	})
	return result, err
}
func (s *Service) JournalEntries(ctx context.Context, tx *gorm.DB, operator, owner, journal string) ([]Entry, error) {
	if !validPortalScope(operator, owner) {
		return nil, ErrScope
	}
	if _, err := uuid.Parse(journal); err != nil {
		return nil, ErrScope
	}
	var result []Entry
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		q := t.Table("ledger_entries e").Select("e.*").Joins("JOIN ledger_journals j ON j.id=e.journal_id").Joins("JOIN ledger_accounts a ON a.id=e.account_id").Where("j.operator_id=? AND a.operator_id=? AND j.id=?", operator, operator, journal).Order("e.id")
		if owner != "" {
			q = q.Where("a.owner_id=?", owner)
		}
		return q.Find(&result).Error
	})
	return result, err
}

type GrantRemaining struct {
	GrantID               string
	PurchasedATG, GiftATG Decimal
}

// GrantRemainders returns exact remaining source balances, including cancellation.
func (s *Service) GrantRemainders(ctx context.Context, tx *gorm.DB, operator, customer string, ids []string) ([]GrantRemaining, error) {
	if !validPortalScope(operator, customer) || customer == "" || len(ids) > 100 {
		return nil, ErrScope
	}
	for _, id := range ids {
		if _, e := uuid.Parse(id); e != nil {
			return nil, ErrScope
		}
	}
	rows := []GrantRemaining{}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Raw(`SELECT src.grant_id,
 COALESCE(sum(CASE WHEN src.kind='paid' THEN src.quantity-src.consumed-src.expired-src.cancelled ELSE 0 END),0)::text AS purchased_atg,
 COALESCE(sum(CASE WHEN src.kind<>'paid' THEN src.quantity-src.consumed-src.expired-src.cancelled ELSE 0 END),0)::text AS gift_atg
 FROM ledger_grant_sources src JOIN ledger_grants g ON g.id=src.grant_id
 WHERE g.operator_id=? AND g.customer_id=? AND g.id IN ? GROUP BY src.grant_id`, operator, customer, ids).Scan(&rows).Error
	})
	return rows, e
}
