package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
	"strings"
)

// A page consists of complete cases; their authorized legs and payment facts
// are appended by refundStaffResource only after the case page is selected.
func (a *Application) refundCasesPage(c *gin.Context, operator string) ([]remediation.Case, string, error) {
	at, id, err := organization.ParseAuditCursor(c.Query("cursor"))
	if err != nil {
		return nil, "", err
	}
	q := a.DB.WithContext(c.Request.Context()).Where("operator_id=?", operator)
	if term := strings.TrimSpace(c.Query("q")); term != "" {
		term = "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(term) + "%"
		q = q.Where(`(id::text ILIKE ? OR customer_id::text ILIKE ? OR order_id::text ILIKE ? OR reason ILIKE ? OR kind ILIKE ? OR EXISTS (SELECT 1 FROM remediation_refund_legs l WHERE l.case_id=remediation_cases.id AND l.operator_id=? AND (l.id::text ILIKE ? OR l.receipt_id::text ILIKE ?)))`, term, term, term, term, term, operator, term, term)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		q = q.Where("state=?", status)
	}
	if at != nil {
		q = q.Where("(created_at,id)<(?,?)", *at, id)
	}
	rows := []remediation.Case{}
	if err = q.Order("created_at DESC,id DESC").Limit(51).Find(&rows).Error; err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > 50 {
		rows = rows[:50]
		last := rows[len(rows)-1]
		next = organization.NextAuditCursor([]organization.AuditEvent{{ID: last.ID, CreatedAt: last.CreatedAt}}, 1)
	}
	return rows, next, nil
}
