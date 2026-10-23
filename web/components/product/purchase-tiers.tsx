"use client";
import { useState } from "react";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { FieldHelp } from "./field-help";

export type PurchaseTier = { from_usd: string; gift_percent: string };
const scale = 10n ** 18n;
function atoms(value: string): bigint {
  if (!/^(0|[1-9][0-9]*)(\.[0-9]{1,18})?$/.test(value)) throw new Error("阶梯金额和赠送百分比须为非负数，最多 18 位小数。");
  const [whole, fraction = ""] = value.split(".");
  const n = BigInt(whole + fraction.padEnd(18, "0"));
  if (n >= 10n ** 38n) throw new Error("金额超出支持范围。");
  return n;
}
function decimal(value: bigint): string {
  const digits = value.toString().padStart(19, "0");
  return `${digits.slice(0, -18)}.${digits.slice(-18)}`.replace(/\.?0+$/, "");
}
export function purchaseTiers(value: unknown): PurchaseTier[] {
  if (!Array.isArray(value) || value.length === 0 || value.length > 100) throw new Error("请至少设置一个赠送阶梯。");
  const tiers = value as PurchaseTier[];
  let previous = -1n;
  tiers.forEach((tier, i) => {
    const from = atoms(tier.from_usd); atoms(tier.gift_percent);
    if (i === 0 && from !== 0n || from <= previous) throw new Error("首阶从 0 USD 开始，后续起点必须依次递增。");
    previous = from;
  });
  return tiers;
}
export function calculatePurchase(tiersValue: unknown, amount: string) {
  const tiers = purchaseTiers(tiersValue), paid = atoms(amount);
  if (paid <= 0n) throw new Error("采购金额必须大于 0。");
  let numerator = 0n;
  for (let i = 0; i < tiers.length; i++) {
    const from = atoms(tiers[i].from_usd);
    if (paid <= from) break;
    const next = i + 1 < tiers.length ? atoms(tiers[i + 1].from_usd) : paid;
    const end = paid < next ? paid : next;
    numerator += (end - from) * atoms(tiers[i].gift_percent);
  }
  const gift = numerator / (100n * scale);
  if (paid + gift >= 10n ** 38n) throw new Error("金额超出支持范围。");
  return { paid: decimal(paid), gift: decimal(gift), total: decimal(paid + gift) };
}
export function PurchaseTierSummary({ value }: { value: unknown }) {
  const { t } = useI18n();
  let tiers: PurchaseTier[];
  try { tiers = purchaseTiers(value); } catch { return <span>{t("历史固定报价，仅供查询")}</span>; }
  return <ul className="space-y-2 text-sm">{tiers.map((tier, i) => <li key={i}>{t(i + 1 < tiers.length ? "{from}–{to} USD 部分赠送 {percent}%" : "超过 {from} USD 的部分赠送 {percent}%", { from: tier.from_usd, to: tiers[i + 1]?.from_usd || "", percent: tier.gift_percent })}</li>)}</ul>;
}
export function PurchasePreview({ tiers, amount }: { tiers: unknown; amount: string }) {
  const { t } = useI18n();
  if (!amount) return null;
  try {
    const result = calculatePurchase(tiers, amount);
    return <div className="rounded-control bg-brand-soft p-4 text-sm leading-7" aria-live="polite"><p>{t("购买 {paid} ATG + 赠送 {gift} ATG = 合计 {total} ATG", result)}</p><p className="text-xs text-ink-secondary">{t("每 1 USD 购买 1 ATG；赠送按本单金额分段累加，不跨订单累计。")}</p></div>;
  } catch { return <p className="text-sm text-ink-mute">{t("填写有效金额和连续阶梯后显示试算结果。")}</p>; }
}
export function PurchaseTiersEditor({ value, disabled, change }: { value: string; disabled: boolean; change: (value: string) => void }) {
  const { t } = useI18n();
  const [amount, setAmount] = useState("2000");
  let tiers: PurchaseTier[];
  try { tiers = JSON.parse(value); if (!Array.isArray(tiers)) throw new Error(); } catch { tiers = [{ from_usd: "0", gift_percent: "0" }]; }
  const update = (i: number, key: keyof PurchaseTier, next: string) => change(JSON.stringify(tiers.map((tier, index) => index === i ? { ...tier, [key]: next } : tier)));
  return <div className="space-y-4"><div className="flex items-center gap-2"><h4 className="text-sm font-semibold">{t("阶梯赠送")}</h4><FieldHelp title={t("阶梯赠送")}>{t("每 1 USD 固定购买 1 ATG。赠送只计算落在本阶区间的金额，填 1 表示 1%，最后一阶没有上限。")}</FieldHelp></div><div className="space-y-3">{tiers.map((tier, i) => <div key={i} className="flex flex-wrap items-end gap-3 rounded-control border border-hairline p-3"><label className="min-w-36 flex-1 text-sm">{t("区间起点 USD")}<Input className="mt-2" inputMode="decimal" value={tier.from_usd} disabled={disabled || i === 0} onChange={e => update(i, "from_usd", e.target.value)} /></label><label className="min-w-36 flex-1 text-sm">{t("本段赠送 %")}<Input className="mt-2" inputMode="decimal" value={tier.gift_percent} disabled={disabled} onChange={e => update(i, "gift_percent", e.target.value)} /></label>{i > 0 && <Button variant="ghost" type="button" disabled={disabled} onClick={() => change(JSON.stringify(tiers.filter((_, index) => index !== i)))}>{t("删除")}</Button>}</div>)}</div><Button variant="outline" type="button" disabled={disabled || tiers.length >= 100} onClick={() => { let next = ""; try { next = decimal(atoms(tiers.at(-1)?.from_usd || "0") + 1000n * scale); } catch { /* Ask for a valid next threshold. */ } change(JSON.stringify([...tiers, { from_usd: next, gift_percent: "0" }])); }}>{t("添加赠送阶梯")}</Button><label className="block text-sm">{t("试算采购金额 USD")}<Input className="mt-2 max-w-64" inputMode="decimal" value={amount} onChange={e => setAmount(e.target.value)} /></label><PurchasePreview tiers={tiers} amount={amount} /></div>;
}
