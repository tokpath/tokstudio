package portal

import (
	"math/big"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func ratePercentage(rate ledger.Decimal) ledger.Decimal {
	value, _ := ledger.Quantize(new(big.Rat).Mul(rate.Rat(), big.NewRat(100, 1)))
	return value
}

func percentageRate(points ledger.Decimal) (ledger.Decimal, error) {
	if points.Sign() < 0 || points.Cmp(ledger.FromInt64(100)) > 0 {
		return ledger.Zero(), incentives.ErrInvalidInput
	}
	rate, remainder := ledger.Quantize(new(big.Rat).Quo(points.Rat(), big.NewRat(100, 1)))
	if remainder.Sign() != 0 {
		return ledger.Zero(), incentives.ErrInvalidInput
	}
	return rate, nil
}

func campaignStatus(v incentives.CampaignVersion, now time.Time) string {
	if !v.Enabled {
		return "disabled"
	}
	if now.Before(v.StartsAt) {
		return "scheduled"
	}
	if !now.Before(v.EndsAt) {
		return "ended"
	}
	return "active"
}

func activityOfferDescription(o incentives.ActivityOffer, locale string) string {
	text := func(source, amount string) string {
		return strings.ReplaceAll(httpx.LocalizedText(locale, source), "{amount}", amount)
	}
	parts := []string{}
	if o.FinalPriceUSD != nil {
		parts = append(parts, text("活动价 {amount} USD", o.FinalPriceUSD.String()))
	}
	if o.Kind == "kol" {
		parts = append(parts, text("按套餐基础额度加赠 {amount}%", ratePercentage(o.ConcessionRate).String()))
	} else if o.GiftATG.Sign() > 0 {
		parts = append(parts, text("额外赠送 {amount} ATG", o.GiftATG.String()))
	}
	return strings.Join(parts, " · ")
}

func promotionFunctionOperation(base, customer string) operation {
	return operation{Name: "function", Label: "管理推广功能", Method: "POST", Path: base, Permission: "marketing.manage", Fields: []field{
		defaultField("action", "办理类型", "hidden", "function"),
		defaultField("customer_id", "当前客户", "hidden", customer),
		selectField("status", "后续分佣", "active|正常应计", "suspended|暂停应计（可恢复）", "terminated|永久停止（不可恢复）"),
		defaultField("release_blocked", "暂缓佣金解冻与使用", "boolean", false),
		f("evidence", "处理说明", "textarea"),
	}, Description: "风险审查可只暂缓解冻与使用，继续正确应计。暂停或永久停止才会阻止后续分佣。", Confirmation: "确认处理所选客户的推广功能？已取得的资格、正确佣金和邀请关系保留；永久停止后不能恢复。"}
}

func (a *Application) marketingResource(c *gin.Context, op organization.Operator, actor, base string, p gin.H) {
	rows, err := a.Incentives.CampaignsForOperator(c.Request.Context(), nil, actor, op.ID)
	if err != nil {
		handleError(c, err)
		return
	}
	var pairs []struct{ MerchantCampaignID, KOLCampaignID string }
	if err = a.DB.WithContext(c.Request.Context()).Table("incentives_combinations").Where("operator_id=?", op.ID).Find(&pairs).Error; err != nil {
		handleError(c, err)
		return
	}
	names := map[string]string{}
	for _, row := range rows {
		names[row.Campaign.ID] = row.Version.Name
	}
	items := []any{}
	for _, row := range rows {
		v := values(row.Version)
		v["kind"], v["owner_customer_id"] = row.Campaign.Kind, row.Campaign.OwnerCustomerID
		v["used_uses"], v["reserved_uses"] = row.Campaign.UsedUses, row.Campaign.ReservedUses
		v["used_gift_atg"], v["used_discount_usd"] = row.Campaign.UsedGiftATG, row.Campaign.UsedDiscountUSD
		v["concession_percent"] = ratePercentage(row.Version.ConcessionRate)
		v["merchant_campaign_id"] = row.Campaign.ID
		combined := []string{}
		for _, pair := range pairs {
			if pair.MerchantCampaignID == row.Campaign.ID {
				combined = append(combined, names[pair.KOLCampaignID])
			}
		}
		v["combined_campaign_names"] = strings.Join(combined, "、")
		status := campaignStatus(row.Version, time.Now())
		r := entity(row.Campaign.ID, status, row.Version.Version, v)
		r["allowed_actions"] = []string{}
		if row.Campaign.Kind == "merchant" {
			r["allowed_actions"] = []string{"publish", "combination"}
		}
		items = append(items, r)
	}
	p["title"] = "套餐营销活动"
	p["description"] = "选择套餐，设置活动价或加赠、时间与名额。KOL 活动由客户自行创建，默认不能与经营方优惠组合。"
	p["items"] = items
	p["columns"] = []gin.H{col("package_id", "适用套餐", "text"), col("final_price_usd", "活动售价", "usd"), col("gift_atg", "额外赠送", "atg"), col("used_uses", "已用名额", "text"), col("reserved_uses", "待付款占用", "text"), col("ends_at", "结束时间", "datetime")}
	p["operations"] = []operation{{Name: "publish", Label: "创建套餐活动", Method: "POST", Path: base, Fields: []field{jsonField("活动名称、套餐、优惠、时间与名额")}}}
	p["row_operations"] = []operation{{Name: "combination", Label: "组合 KOL 优惠", Method: "POST", Path: base, Fields: []field{
		defaultField("action", "办理类型", "hidden", "combination"), f("merchant_campaign_id", "当前经营方活动", "hidden"), f("kol_campaign_id", "同套餐 KOL 活动", "text"), defaultField("enabled", "允许与此 KOL 活动组合", "boolean", true),
	}, Description: "只开放所选活动这一对组合。已开放的活动列在当前活动详情中；选择同一活动并关闭允许，即可取消组合。"}}
	a.resourceResponse(c, p)
}
