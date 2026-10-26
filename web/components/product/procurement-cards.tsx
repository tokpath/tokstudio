"use client";

import type { ReactNode } from "react";
import type { Entity } from "@/lib/contracts";
import { useI18n } from "@/components/i18n";
import { Card } from "@/components/ui/card";
import { Empty, Money, Status, When } from "./shared";
import { PurchaseTierSummary } from "./purchase-tiers";

export function ProcurementCards({rows, title, actions}: {rows: Entity[]; title: (row: Entity) => string; actions: (row: Entity) => ReactNode}) {
  const {t} = useI18n();
  if (!rows.length) return <Empty>{t("当前筛选下没有记录。")}</Empty>;
  return <div className="space-y-4">{rows.map(row => {
    const v = row.values, kind = String(v.record_kind || ""), quote = kind === "quote", authorization = kind === "authorization", attempt = kind === "replenishment_attempt";
    const amount = String(v.due_usd ?? v.amount_usd ?? "0");
    const status = authorization ? v.enabled === false ? "disabled" : "authorization" : row.status || "";
    return <Card key={row.id}>
      <div className="flex flex-wrap items-start justify-between gap-3"><div><h3 className="text-lg font-semibold">{title(row)}</h3><p className="mt-1 text-sm text-ink-secondary">{String(v.supplier_name || "")}{v.supplier_name && v.buyer_name ? " → " : ""}{String(v.buyer_name || "")}</p></div><Status value={status} /></div>
      {quote ? <div className="mt-4 space-y-3"><p className="text-sm">{t("每 1 USD 购买 1 ATG，赠送按本单金额分段累加。")}</p><PurchaseTierSummary value={v.gift_tiers} /><p className="text-xs text-ink-secondary">{String(v.exit_policy_summary || "")}</p>{v.valid_until && <p className="text-xs text-ink-mute">{t("报价截止时间")} <When value={String(v.valid_until)} /></p>}</div> : <dl className="mt-5 grid gap-4 sm:grid-cols-3">
        <Amount label={authorization ? t("每次采购USD") : t("采购金额")} value={amount} unit="USD" />
        {authorization ? <><Amount label={t("低于此采购余额时补货")} value={String(v.trigger_available_atg || "0")} unit="ATG" /><Amount label={t("周期最高USD")} value={String(v.period_limit_usd || "0")} unit="USD" /></> : !attempt ? <><Amount label={t("购买库存")} value={String(v.paid_atg || "0")} unit="ATG" /><Amount label={t("赠送库存")} value={String(v.gift_atg || "0")} unit="ATG" /></> : <><div><dt className="text-xs text-ink-mute">{t("发起时间")}</dt><dd className="mt-2 text-sm"><When value={String(v.created_at || "")} /></dd></div>{v.error_code && <div><dt className="text-xs text-ink-mute">{t("处理提示")}</dt><dd className="mt-2 text-sm">{String(v.error_code)}</dd></div>}</>}
      </dl>}
      {authorization && <p className="mt-4 text-sm text-ink-secondary">{t(({month:"每月",quarter:"每季",year:"每年"} as Record<string,string>)[String(v.period)] || "每月")} · {t(v.mode === "automatic" ? "Stripe 自动扣款" : "自动建单，人工付款")}</p>}
      {v.reserved_note && <div className={row.status === "awaiting_payment" || row.status === "paid" ? "mt-4 text-sm text-danger" : "mt-4 text-xs text-ink-mute"}><p>{String(v.reserved_note)}</p>{String(v.reserved_atg || "0") !== "0" && <p className="mt-2">{t("占用供货库存")} <Money value={String(v.reserved_atg)} unit="ATG" /></p>}</div>}
      {row.status === "awaiting_payment" && <p className="mt-4 text-sm text-hold">{t("付款后自动发放采购额度；未付款订单可取消。")}</p>}
      {row.status === "paid" && <p className="mt-4 text-sm text-hold">{t("款项已确认，正在完成额度交付。")}</p>}
      {(v.terms_summary || v.contract_url) && <details className="mt-4 border-t border-hairline pt-3 text-sm"><summary className="cursor-pointer text-ink-secondary">{t("补充约定与合同")}</summary>{v.terms_summary && <p className="mt-3 whitespace-pre-wrap leading-7">{String(v.terms_summary)}</p>}{v.contract_url && <a className="mt-3 inline-block text-brand-emphasis underline" href={String(v.contract_url)} target="_blank" rel="noreferrer">{t("查看合同")}</a>}</details>}
      <div className="mt-5 border-t border-hairline pt-3">{actions(row)}</div>
    </Card>;
  })}</div>;
}

function Amount({label, value, unit}: {label: string; value: string; unit: string}) {
  return <div><dt className="text-xs text-ink-mute">{label}</dt><dd className="mt-2 text-xl font-semibold"><Money value={value} unit={unit} /></dd></div>;
}
