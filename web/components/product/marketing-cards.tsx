"use client";

import type { ReactNode } from "react";
import type { Entity } from "@/lib/contracts";
import { useI18n } from "@/components/i18n";
import { Card } from "@/components/ui/card";
import { Money, Status, When } from "./shared";

export function MarketingCard({ row, children, packageName, ownerName }: { row: Entity; children: ReactNode; packageName: string; ownerName?: string }) {
  const { t } = useI18n();
  const v = row.values, kol = v.kind === "kol";
  return <Card className="min-w-0" data-campaign-id={row.id}>
    <div className="flex items-start justify-between gap-3"><div><p className="mb-2 text-xs text-ink-mute">{t(kol ? "KOL 推广" : "经营方活动")}{kol && ownerName ? ` · ${ownerName}` : ""}</p><h3 className="font-semibold">{String(v.name || packageName)}</h3><p className="mt-2 text-sm text-ink-secondary">{packageName}</p></div><Status value={row.status || "active"} /></div>
    <div className="my-5 flex flex-wrap gap-x-6 gap-y-3 text-lg font-semibold text-brand-emphasis">
      {kol ? <span>{t("让利 {points} 个百分点", { points: String(v.concession_percent) })}</span> : <>
        {v.final_price_usd != null && <span><Money value={String(v.final_price_usd)} unit="USD" /></span>}
        {String(v.gift_atg || "0") !== "0" && <span>+<Money value={String(v.gift_atg)} unit="ATG" /></span>}
      </>}
    </div>
    <p className="text-sm text-ink-secondary">{t(({ any: "所有符合条件的客户", first_order: "首次购买客户", existing: "已有购买记录客户" } as Record<string, string>)[String(v.eligibility)] || "所有符合条件的客户")}</p>
    <p className="mt-2 text-xs leading-6 text-ink-mute"><When value={String(v.starts_at)} /> — <When value={String(v.ends_at)} /></p>
    <p className="mt-3 text-sm">{t("每位客户最多 {count} 次", { count: String(v.per_customer_limit) })} · {Number(v.total_use_limit) > 0 ? t("共 {count} 个名额", { count: String(v.total_use_limit) }) : t("不单独限制总名额")}</p>
    <p className="mt-2 text-xs text-ink-mute">{t("已用 {used}，待付款占用 {reserved}", { used: String(v.used_uses || 0), reserved: String(v.reserved_uses || 0) })}</p>
    {!kol && <p className="mt-3 text-xs leading-6 text-ink-secondary">{v.combined_campaign_names ? t("已开放组合：{names}", { names: String(v.combined_campaign_names) }) : t("未开放与 KOL 优惠组合")}</p>}
    <div className="mt-5 border-t border-hairline pt-3">{children}</div>
  </Card>;
}

export function ReferralPolicyCard({ row, children }: { row: Entity; children: ReactNode }) {
  const { t } = useI18n();
  const v = row.values;
  return <Card>
    <div className="grid gap-6 md:grid-cols-3">
      <section><h3 className="font-semibold">{t("注册赠送")}</h3><p className="mt-4 text-sm">{t("受邀客户")} · <Money value={String(v.invitee_atg)} unit="ATG" /></p><p className="mt-2 text-sm">{t("普通邀请人")} · <Money value={String(v.inviter_atg)} unit="ATG" /></p><p className="mt-3 text-xs leading-6 text-ink-mute">{t("有效 {days} 天；KOL 邀请人不再获得注册赠送。", { days: String(v.reward_validity_days) })}</p></section>
      <section><h3 className="font-semibold">{t("成为 KOL")}</h3><p className="mt-4 text-lg font-semibold"><Money value={String(v.threshold_usd)} unit="USD" /></p><p className="mt-3 text-xs leading-6 text-ink-mute">{t("本人已确认的真实支出达标后自动取得资格，赠送不计入。提高门槛保留已有资格。")}</p></section>
      <section><h3 className="font-semibold">{t("两级佣金")}</h3><p className="mt-4 text-sm">{t("直接邀请佣金 %")} · {String(v.direct_percent)}%</p><p className="mt-2 text-sm">{t("间接邀请佣金 %")} · {String(v.indirect_percent)}%</p><p className="mt-3 text-xs leading-6 text-ink-mute">{t("有偿调用确认等待 {days} 天，确认后计入资格并解冻佣金。", { days: String(v.confirmation_days) })}</p><p className="mt-2 text-xs text-ink-mute">{Number(v.kol_cross_activity_limit) > 0 ? t("同一 KOL 跨活动每人最多 {count} 次", { count: String(v.kol_cross_activity_limit) }) : t("同一 KOL 跨活动不单独限次")}</p></section>
    </div>
    <div className="mt-6 border-t border-hairline pt-4">{children}</div>
  </Card>;
}
