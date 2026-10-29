package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"time"
)

func reportRange(c *gin.Context) (time.Time, time.Time, error) {
	to := time.Now().UTC()
	from := time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, time.UTC)
	var e error
	if c.Query("from") != "" {
		from, e = time.Parse(time.RFC3339, c.Query("from"))
		if e != nil {
			return from, to, operations.ErrInvalid
		}
	}
	if c.Query("to") != "" {
		to, e = time.Parse(time.RFC3339, c.Query("to"))
		if e != nil {
			return from, to, operations.ErrInvalid
		}
	}
	return from, to, nil
}
func (a *Application) financialReport(c *gin.Context) {
	op, actor, ok := a.staffScope(c, "reports")
	if !ok {
		return
	}
	from, to, e := reportRange(c)
	if e != nil {
		handleError(c, e)
		return
	}
	report, e := a.Operations.FinancialReport(c.Request.Context(), actor, op.ID, from, to)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, report)
}
func (a *Application) financialResource(c *gin.Context, op organization.Operator, actor string, p gin.H) {
	from, to, e := reportRange(c)
	if e != nil {
		handleError(c, e)
		return
	}
	report, e := a.Operations.FinancialReport(c.Request.Context(), actor, op.ID, from, to)
	if e != nil {
		handleError(c, e)
		return
	}
	p["title"] = "经营财务事实"
	p["description"] = "同一数据库快照列示清算币种业务事实与各币种原始收支。供应商参考、报告与登记成本分别呈现，不折算，不计算跨币种利润。"
	p["items"] = []any{}
	p["summary"] = []gin.H{{"label": "已确认收入", "value": report.Revenue.TotalMoney.String(), "unit": report.SettlementCurrency}, {"label": "清算币种业务成本", "value": report.Costs.TotalMoney.String(), "unit": report.SettlementCurrency}}
	p["report"] = report
	a.resourceResponse(c, p)
}
