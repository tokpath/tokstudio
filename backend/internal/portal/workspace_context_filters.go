package portal

import (
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

type customerSectionFilters struct {
	Statuses []string `json:"statuses,omitempty"`
	Units    []string `json:"units,omitempty"`
}

type customerSectionSearch struct {
	dateColumn   string
	statusColumn string
	keywordSQL   string // Only fields already visible within this section's read scope.
	filters      customerSectionFilters
}

var customerSectionSearches = map[string]customerSectionSearch{
	"orders": {
		dateColumn: "created_at", statusColumn: "status",
		keywordSQL: `id::text, kind, CASE kind WHEN 'package' THEN '套餐购买' WHEN 'renewal' THEN '套餐续费' WHEN 'topup' THEN '余额充值' END,
			(SELECT p.name FROM commerce_package_quotes q JOIN commerce_packages p ON p.id=q.package_id
			 WHERE q.id=commerce_orders.quote_id)`,
		filters: customerSectionFilters{Statuses: []string{"pending", "partial", "paid", "completed", "closed", "exception"}},
	},
	"assets": {
		dateColumn: "created_at",
		keywordSQL: `id::text, purpose, unit, CASE
			WHEN purpose LIKE 'customer:atg:%' THEN '客户 ATG 权益'
			WHEN purpose='customer:money' THEN '支付余额 现金余额'
			WHEN purpose='commission:available' THEN '可用佣金'
			WHEN purpose='commission:frozen' THEN '冻结佣金' END`,
		filters: customerSectionFilters{Units: []string{"ATG", "MONEY"}},
	},
	"requests": {
		dateColumn: "accepted_at", statusColumn: "status",
		keywordSQL: "id::text, model_id, operation",
		filters:    customerSectionFilters{Statuses: []string{"accepted", "executing", "submitted", "awaiting_usage", "completed", "failed", "free", "cancelled"}},
	},
	"withdrawals": {
		dateColumn: "created_at", statusColumn: "status",
		keywordSQL: "id::text, method_name, recipient_reference, payment_reference",
		filters:    customerSectionFilters{Statuses: []string{"requested", "approved", "paid", "cancelled", "failed"}},
	},
	"invoices": {
		dateColumn: "created_at", statusColumn: "state",
		keywordSQL: `id::text, invoice_reference,
			(SELECT string_agg(src.order_id::text, ' ') FROM operations_invoice_orders src WHERE src.invoice_id=operations_invoices.id)`,
		filters: customerSectionFilters{Statuses: []string{"requested", "issued", "rejected", "review_required", "voided"}},
	},
	"refunds": {
		dateColumn: "created_at", statusColumn: "state",
		keywordSQL: "id::text, order_id::text, kind, reason",
		filters:    customerSectionFilters{Statuses: []string{"pending_review", "approved", "completed", "rejected", "cancelled", "exception"}},
	},
}

type customerContextQuery struct {
	section, keyword, status, unit, cursorID string
	from, to, cursorAt                       *time.Time
}

func parseCustomerContextQuery(c *gin.Context) (customerContextQuery, bool) {
	q := customerContextQuery{
		section: c.Query("section"), keyword: strings.TrimSpace(c.Query("q")), status: c.Query("status"), unit: c.Query("unit"),
	}
	cursor, from, to := c.Query("cursor"), c.Query("from"), c.Query("to")
	spec, searchable := customerSectionSearches[q.section]
	hasFilters := q.keyword != "" || q.status != "" || q.unit != "" || from != "" || to != ""
	invalidSection := !searchable && (hasFilters || q.section != "commissions" && (q.section != "" || cursor != ""))
	if invalidSection || !utf8.ValidString(q.keyword) || strings.ContainsRune(q.keyword, 0) || utf8.RuneCountInString(q.keyword) > 200 ||
		q.status != "" && !slices.Contains(spec.filters.Statuses, q.status) || q.unit != "" && !slices.Contains(spec.filters.Units, q.unit) {
		fail(c, 400, "invalid_input", "请检查输入内容。", false)
		return q, false
	}
	start, startErr := customerContextDate(from)
	end, endErr := customerContextDate(to)
	if startErr != nil || endErr != nil || (start != nil && end != nil && !start.Before(*end)) {
		fail(c, 400, "invalid_input", "请选择有效日期范围。", false)
		return q, false
	}
	q.from, q.to = start, end
	if cursor != "" {
		if !searchable {
			return q, validContextCursor(c, cursor)
		}
		var err error
		q.cursorAt, q.cursorID, err = organization.ParseAuditCursor(cursor)
		if err != nil {
			fail(c, 400, "invalid_cursor", "分页位置无效，请重新查询", false)
			return q, false
		}
	}
	return q, true
}

func customerContextDate(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	at, err := time.Parse(time.RFC3339, raw)
	return &at, err
}

func customerContextRows[T any](db *gorm.DB, resource string, q customerContextQuery, position func(T) (time.Time, string)) ([]T, string, error) {
	spec := customerSectionSearches[resource]
	if q.keyword != "" {
		db = db.Where("strpos(lower(concat_ws(' ', "+spec.keywordSQL+")), lower(?)) > 0", q.keyword)
	}
	if q.status != "" {
		if resource == "refunds" && q.status == "approved" {
			// Both states share the same visible “awaiting payment” status.
			db = db.Where("state IN ?", []string{"approved", "paying"})
		} else {
			db = db.Where(spec.statusColumn+" = ?", q.status)
		}
	}
	if q.unit != "" {
		db = db.Where("unit = ?", q.unit)
	}
	if q.from != nil {
		db = db.Where(spec.dateColumn+" >= ?", *q.from)
	}
	if q.to != nil {
		db = db.Where(spec.dateColumn+" < ?", *q.to)
	}
	if q.cursorAt != nil {
		db = db.Where("("+spec.dateColumn+",id) < (?,?::uuid)", *q.cursorAt, q.cursorID)
	}
	var rows []T
	if err := db.Order(spec.dateColumn + " DESC,id DESC").Limit(workspacePageSize + 1).Find(&rows).Error; err != nil {
		return nil, "", err
	}
	if len(rows) <= workspacePageSize {
		return rows, "", nil
	}
	rows = rows[:workspacePageSize]
	at, id := position(rows[len(rows)-1])
	next := organization.NextAuditCursor([]organization.AuditEvent{{ID: id, CreatedAt: at}}, 1)
	return rows, next, nil
}
