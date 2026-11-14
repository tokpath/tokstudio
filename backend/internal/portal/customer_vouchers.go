package portal

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"mime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"gorm.io/gorm"
)

func customerCSV(c *gin.Context, filename string, header []string, rows [][]string) {
	var b bytes.Buffer
	b.WriteString("\ufeff")
	w := csv.NewWriter(&b)
	locale := httpx.RequestLocale(c.Request)
	for i, label := range header {
		header[i] = httpx.LocalizedText(locale, label)
	}
	_ = w.Write(header)
	for _, row := range rows {
		for i := range row {
			row[i] = operations.SafeCSVCell(row[i])
		}
		_ = w.Write(row)
	}
	w.Flush()
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename + ".csv"}))
	c.Data(200, "text/csv; charset=utf-8", b.Bytes())
}

func statementTime(at *time.Time) string {
	if at == nil {
		return ""
	}
	return at.UTC().Format(time.RFC3339Nano)
}

func (a *Application) customerOrderVoucher(c *gin.Context) {
	ctx, customer := c.Request.Context(), subject(c).Customer.Customer
	order, err := a.Commerce.GetOrder(ctx, nil, customer.ID, c.Param("id"))
	if err != nil || order.BrandID != customer.BrandID {
		if err != nil {
			handleError(c, err)
		} else {
			c.Status(404)
		}
		return
	}
	issuer, err := a.Organization.GetOperator(ctx, nil, order.OriginalOperatorID)
	if err != nil {
		handleError(c, err)
		return
	}
	merchant := issuer.LegalName
	if merchant == "" {
		merchant = issuer.Name
	}
	currency := order.MonetarySnapshot.Currency
	if currency == "" {
		settings, err := ledger.GetMonetary(ctx, a.DB)
		if err != nil {
			handleError(c, err)
			return
		}
		currency = settings.Currency
	}
	kind := c.Param("kind")
	switch kind {
	case "order":
		paid := order.ExternalAllocatedMoney
		if order.InternalStatus == "captured" {
			paid = paid.Add(order.InternalMoney).Add(order.CommissionMoney)
		}
		customerCSV(c, "order-"+order.ID, []string{"订单编号", "实际经营方", "业务类型", "状态", "订单本金", "已核实本金", "币种", "创建时间", "交付时间"},
			[][]string{{order.ID, merchant, customerExportLabel(c, "kind", order.Kind), customerExportLabel(c, "status", order.Status), order.DueMoney.String(), paid.String(), currency, statementTime(&order.CreatedAt), statementTime(order.CompletedAt)}})
	case "receipt":
		var receipts []struct {
			ID, Currency, OriginalAmount string
			PaidAt                       *time.Time
			ObservedAt                   time.Time
			AmountMoney                  ledger.Decimal
		}
		err := a.DB.WithContext(ctx).Raw(`SELECT r.id,r.currency,r.original_amount,r.paid_at,r.observed_at,a.amount_money
 FROM commerce_receipt_allocations a JOIN commerce_receipts r ON r.id=a.receipt_id AND r.operator_id=a.operator_id
 WHERE a.order_id=? AND a.operator_id=? ORDER BY r.observed_at,r.id`, order.ID, order.OriginalOperatorID).Scan(&receipts).Error
		if err != nil {
			handleError(c, err)
			return
		}
		rows := make([][]string, 0, len(receipts))
		for _, r := range receipts {
			rows = append(rows, []string{r.ID, order.ID, merchant, r.OriginalAmount, r.Currency, r.AmountMoney.String(), currency, statementTime(r.PaidAt), statementTime(&r.ObservedAt)})
		}
		if len(rows) == 0 {
			c.Status(404)
			return
		}
		customerCSV(c, "receipt-"+order.ID, []string{"收款编号", "订单编号", "实际经营方", "实际原币收款总额", "实际币种", "已核实本金", "清算币种", "实际付款时间", "登记时间"}, rows)
	default:
		c.Status(404)
	}
}

func (a *Application) customerRefundVoucher(c *gin.Context) {
	ctx, customer := c.Request.Context(), subject(c).Customer.Customer
	ref := c.Param("id")
	if _, err := uuid.Parse(ref); err != nil {
		handleError(c, operations.ErrInvalid)
		return
	}
	var refund struct {
		ID, OrderID, LegalName, Currency string
		CompletedAt                      time.Time
		AmountMoney                      ledger.Decimal
	}
	result := a.DB.WithContext(ctx).Raw(`SELECT r.id,COALESCE(r.order_id::text,'') AS order_id,COALESCE(NULLIF(o.legal_name,''),o.name) AS legal_name,
 COALESCE(NULLIF(original.monetary_snapshot->>'currency',''),m.currency) AS currency,r.amount_money,r.completed_at
 FROM remediation_cases r JOIN organization_operators o ON o.id=r.operator_id AND o.brand_id=?
 LEFT JOIN commerce_orders original ON original.id=r.order_id
 JOIN ledger_monetary_settings m ON m.id='platform'
 WHERE r.id=? AND r.customer_id=? AND r.state='completed' AND r.completed_at IS NOT NULL`, customer.BrandID, ref, customer.ID).Scan(&refund)
	if result.Error != nil {
		handleError(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		c.Status(404)
		return
	}
	customerCSV(c, "refund-"+refund.ID, []string{"退款编号", "订单编号", "实际经营方", "退款金额", "币种", "完成时间"},
		[][]string{{refund.ID, refund.OrderID, refund.LegalName, refund.AmountMoney.String(), refund.Currency, statementTime(&refund.CompletedAt)}})
}

func (a *Application) customerStatement(c *gin.Context) {
	customer := subject(c).Customer.Customer
	from, to, err := reportRange(c)
	if err != nil || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		handleError(c, operations.ErrInvalid)
		return
	}
	var records []struct {
		ID, Kind, Unit, SourceID, LegalName string
		CreatedAt                           time.Time
		Amount, BalanceAfter                ledger.Decimal
	}
	var currency string
	err = a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		monetary, err := ledger.GetMonetary(c.Request.Context(), tx)
		if err != nil {
			return err
		}
		currency = monetary.Currency
		return tx.Raw(`SELECT j.id,j.kind,e.unit,j.source_id,COALESCE(NULLIF(o.legal_name,''),o.name) AS legal_name,e.created_at,e.amount::text,e.balance_after::text
 FROM ledger_accounts a JOIN ledger_entries e ON e.account_id=a.id
 JOIN ledger_journals j ON j.id=e.journal_id JOIN organization_operators o ON o.id=a.operator_id AND o.brand_id=?
 WHERE a.owner_id=? AND (a.purpose IN ('customer:money','commission:available','commission:frozen')
 OR a.purpose LIKE 'customer:atg:%') AND e.created_at>=? AND e.created_at<?
 ORDER BY e.created_at,e.id LIMIT 10001`, customer.BrandID, customer.ID, from, to).Scan(&records).Error
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		handleError(c, err)
		return
	}
	if len(records) > 10000 {
		handleError(c, operations.ErrTooLarge)
		return
	}
	rows := make([][]string, 0, len(records))
	for _, row := range records {
		unit := row.Unit
		if unit == ledger.MoneyUnit {
			unit = currency
		}
		rows = append(rows, []string{statementTime(&row.CreatedAt), row.LegalName, customerExportLabel(c, "kind", row.Kind), unit, row.Amount.String(), row.BalanceAfter.String(), row.ID, row.SourceID})
	}
	customerCSV(c, "statement", []string{"入账时间", "实际经营方", "账务事项", "单位", "变动金额", "变动后余额", "账务记录编号", "来源记录编号"}, rows)
}

func customerExportLabel(c *gin.Context, key, value string) string {
	return httpx.LocalizedText(httpx.RequestLocale(c.Request), operations.ExportLabel(key, value))
}
