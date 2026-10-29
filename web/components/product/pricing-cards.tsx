"use client";

import type { ReactNode } from "react";
import { useI18n } from "@/components/i18n";
import type { Entity, JSONValue } from "@/lib/contracts";
import { billingLabel } from "@/lib/model-billing";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Money, When } from "./shared";
import { totalCredits, planPeriods } from "./customer-finance";
import { groupResolutionPrices } from "./operation-config";

const object = (value: JSONValue | undefined): Record<string, JSONValue> => value && typeof value === "object" && !Array.isArray(value) ? value : {};
const units: Record<string, string> = { token: "Token", character: "字符", image: "张", megapixel: "百万像素", second: "秒" };

export function PriceRates({ components }: { components: JSONValue | undefined }) {
  const { t } = useI18n();
  const items: Record<string, JSONValue>[] = (Array.isArray(components) ? components.map(object) : []).flatMap(item => [item, ...(Array.isArray(item.resolutions) ? item.resolutions.map(row => { const resolution = object(row); return { ...item, label: `${String(item.label || item.key)} · ${String(resolution.size)}`, rate: resolution.rate, resolutions: [] }; }) : [])]);
  if (!items.length) return <p className="text-sm text-hold">{t("价格未配齐，暂不可调用")}</p>;
  return <ul className="space-y-2 text-sm">{items.map((item, index) => <li key={index} className="flex flex-wrap justify-between gap-x-4 gap-y-1"><span className="text-ink-secondary">{billingLabel(String(item.label || item.component || item.key), t)}</span><span><Money value={String(item.rate ?? item.amount_atg ?? "0")} /> / {String(item.per || "1")} {t(units[String(item.unit)] || String(item.unit))}</span></li>)}</ul>;
}

export function ModelPricingCard({ row, canPrice, canTemplate, onPrice, onTemplate }: { row: Entity; canPrice: boolean; canTemplate: boolean; onPrice: (price?: Record<string, JSONValue>) => void; onTemplate: () => void }) {
  const { t } = useI18n();
  const books = Array.isArray(row.values.price_books) ? row.values.price_books.map(object) : [];
  const pending = Array.isArray(row.values.pending_changes) ? row.values.pending_changes.map(object) : [];
  const meterLabels = new Map([...(Array.isArray(row.values.current_pricing_components) ? row.values.current_pricing_components.map(object) : []), ...(Array.isArray(row.values.pricing_components) ? row.values.pricing_components.map(object) : [])].map(meter => [meter.key, meter.label]));
  const isListed = !["not_listed", "disabled"].includes(row.status || "");
  return <Card className="min-w-0"><div className="flex flex-wrap items-start justify-between gap-3"><div><h3 className="text-lg font-semibold">{String(row.values.name || row.id)}</h3><p className="mt-1 text-xs text-ink-mute">{t(row.status === "disabled" ? "平台已停用" : !isListed ? "尚未上架，可先准备计费与价格" : row.status === "pricing_unavailable" ? "价格未配齐，暂不可调用" : "已上架")}</p></div><div className="flex flex-wrap gap-2">{canTemplate && <Button size="sm" variant="outline" onClick={onTemplate}>{t("计费模板")}</Button>}{canPrice && <Button size="sm" onClick={() => onPrice()}>{t("设置价格")}</Button>}</div></div>
    {row.values.billing_pending === true && <p className="mt-4 text-xs leading-6 text-hold">{t("计费模板正在更新，请按新模板配齐价格；已有请求继续使用原快照。")}</p>}
    <div className="mt-5 space-y-4">{!books.length ? <p className="rounded-control bg-canvas p-4 text-sm text-ink-secondary">{t("尚未定价。先确认计费模板，再填写本模型的 ATG 单价。")}</p> : books.map((book, index) => {
      const price = object(book.price), scope = object(price.scope), rule = object(price.rule);
      const components = Array.isArray(rule.components) ? rule.components.map(object).filter(meter => meterLabels.has(meter.key)).map(meter => ({ ...meter, label: meterLabels.get(meter.key)! })) : [];
      return <section key={index} className="rounded-control border border-hairline p-4"><div className="mb-3 flex flex-wrap items-center justify-between gap-2"><h4 className="text-sm font-medium">{scope.kind === "supply" ? `${t("供货价")} · ${String(book.name)}` : t("客户零售价")} · {t(price.layer === "promotion" ? "限时促销价" : "常规价格")}{price.model_id === "*" && ` · ${t("全模型默认")}`}</h4><span className="text-xs text-ink-mute">{t(book.status === "scheduled" ? "待生效" : "已生效")}</span></div>{rule.mode === "follow" ? <p className="text-sm text-ink-secondary">{t("跟随上级供货价")} × {String(rule.multiplier)}</p> : <PriceRates components={components} />}<div className="mt-3 flex flex-wrap items-center justify-between gap-2 border-t border-hairline pt-3 text-xs text-ink-mute"><span>{t("生效时间")} <When value={String(price.effective_at)} />{price.ends_at && <> — <When value={String(price.ends_at)} /></>}</span>{canPrice && price.layer === "regular" && <Button size="sm" variant="ghost" onClick={() => onPrice(price)}>{t("调整价格")}</Button>}</div></section>;
    })}</div>
    {pending.length > 0 && <div className="mt-4 space-y-2 rounded-control bg-canvas p-3 text-xs leading-6 text-ink-secondary">{pending.map((change, index) => <p key={index}>{t(change.needs_attention ? "变更尚未完成，请检查配置后重新保存。当前有效配置继续使用。" : "变更已保存，系统正在通知并安排生效，当前请求仍按原配置结算。")}</p>)}</div>}
  </Card>;
}

export function PackageCard({ row, children }: { row: Entity; children: ReactNode }) {
  const { t } = useI18n();
  const v = row.values, total = totalCredits(String(v.paid_atg || "0"), String(v.gift_atg || "0"));
  return <Card className="min-w-0"><div className="flex flex-wrap items-start justify-between gap-3"><h3 className="text-lg font-semibold">{String(v.name)}</h3><span className="rounded-full bg-canvas px-3 py-1 text-xs">{t(v.new_purchase_enabled ? "在售" : "已停止新购")}</span></div><div className="my-5 flex flex-wrap items-end justify-between gap-3"><div><p className="mb-1 text-xs text-ink-mute">{t("合计服务额度")}</p><p className="text-2xl font-semibold"><Money value={total} /></p></div><p className="text-lg font-medium"><Money value={String(v.price_money)} unit="MONEY" /></p></div><p className="text-sm text-ink-secondary">{t("购买额度")} <Money value={String(v.paid_atg)} /> · {t("赠送额度")} <Money value={String(v.gift_atg || "0")} /></p><div className="my-4 flex flex-wrap gap-3 border-y border-hairline py-3 text-sm"><span>{t(planPeriods[String(v.period) === "lifetime" ? "unlimited" : String(v.period)] || String(v.period))}</span><span className="text-ink-mute">{t(v.period === "lifetime" ? "无需续费" : v.renewal_enabled ? "可由客户授权续费" : "已停止续费")}</span></div>{children}</Card>;
}

export function priceDraft(row: Entity, saved?: Record<string, JSONValue>): JSONValue {
  const price = saved || { model_id: row.id, scope: { kind: "retail" }, layer: "regular", rule: { mode: "manual" } };
  const rule = object(price.rule);
  const meters = Array.isArray(row.values.pricing_components) ? row.values.pricing_components.map(object) : [];
  const existing = new Map((Array.isArray(rule.components) ? rule.components.map(object) : []).map(component => [component.key, component]));
  const components = meters.map(meter => ({ ...meter, rate: existing.get(meter.key)?.rate ?? "" }));
  // Each save creates a new version. Do not reuse a saved ID or timestamp.
  return { action: "price", price: { model_id: price.model_id || row.id, scope: price.scope || { kind: "retail" }, layer: "regular", rule: rule.mode === "follow" ? rule : { mode: "manual", components: groupResolutionPrices(components) } } };
}
