package portal

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
)

func executionValuationFields(_ ...string) []field {
	amount := optional("settlement_amount", "本次已核实清偿金额（结算币种，跨币必填）", "decimal")
	amount.Help = "只确认这笔真实付款对应的清偿金额。同币直接按实际付款金额清偿；跨币不能按1:1自动入账。"
	return []field{amount, optional("settlement_evidence", "本笔付款与清偿金额关联凭证（跨币必填）", "textarea")}
}
func (a *Application) refundSettlementConfirmation(ctx context.Context, scope string, amount ledger.Decimal, evidence string) (payments.Valuation, error) {
	settings, e := ledger.GetMonetary(ctx, a.DB)
	if e != nil {
		return payments.Valuation{}, e
	}
	return payments.EnsureValuationIdentity(payments.Valuation{Currency: settings.Currency, Amount: amount, Source: evidence}, scope, settings.Currency), nil
}
