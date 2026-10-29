"use client";
import { useState } from "react";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { FieldHelp } from "./field-help";
import { useMonetary } from "./monetary";

export type PurchaseTier = { from_money: string; gift_percent: string };
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
    const from = atoms(tier.from_money); atoms(tier.gift_percent);
    if (i === 0 && from !== 0n || from <= previous) throw new Error("首阶从 0 开始，后续起点必须依次递增。");
    previous = from;
  });
  return tiers;
}
export function calculatePurchase(tiersValue: unknown, amount: string, atgPerUnit = "1") {
  const tiers = purchaseTiers(tiersValue), amountMoney = atoms(amount), rate = atoms(atgPerUnit);
  if (rate <= 0n) throw new Error("ATG 基础比例必须大于 0。");
  const paid = amountMoney * rate / scale;
  if (amountMoney <= 0n) throw new Error("采购金额必须大于 0。");
  if (paid <= 0n) throw new Error("采购金额过小，换算后不足最小 ATG 单位。");
  let numerator = 0n;
  for (let i = 0; i < tiers.length; i++) {
    const from = atoms(tiers[i].from_money);
    if (amountMoney <= from) break;
    const next = i + 1 < tiers.length ? atoms(tiers[i + 1].from_money) : amountMoney;
    const end = amountMoney < next ? amountMoney : next;
    numerator += (end - from) * atoms(tiers[i].gift_percent);
  }
  const gift = numerator * rate / (100n * scale * scale);
  if (paid + gift >= 10n ** 38n) throw new Error("金额超出支持范围。");
  return { paid: decimal(paid), gift: decimal(gift), total: decimal(paid + gift) };
}
export function PurchaseTierSummary({ value, currency }: { value: unknown; currency?: string }) {
  const { t } = useI18n();
  const monetary = useMonetary();
  let tiers: PurchaseTier[];
  try { tiers = purchaseTiers(value); } catch { return <span>{t("历史固定报价，仅供查询")}</span>; }
  return <ul className="space-y-2 text-sm">{tiers.map((tier, i) => <li key={i}>{t(i + 1 < tiers.length ? "{from}–{to} {currency} 部分赠送 {percent}%" : "超过 {from} {currency} 的部分赠送 {percent}%", { currency: currency || monetary.currency, from: tier.from_money, to: tiers[i + 1]?.from_money || "", percent: tier.gift_percent })}</li>)}</ul>;
}
export function PurchasePreview({ tiers, amount }: { tiers: unknown; amount: string }) {
  const { t } = useI18n();
  const monetary = useMonetary();
  if (!amount) return null;
  if (monetary.status === "unconfigured") return <p className="text-sm text-hold">{t("基础比例尚未生效，暂不提供采购换算。")}</p>;
  try {
    const result = calculatePurchase(tiers, amount, monetary.atg_per_unit);
    return <div className="rounded-control bg-brand-soft p-4 text-sm leading-7" aria-live="polite"><p>{t("购买 {paid} ATG + 赠送 {gift} ATG = 合计 {total} ATG", result)}</p><p className="text-xs text-ink-secondary">{t("每 1 {currency} 购买 {ratio} ATG；赠送按本单金额分段累加，不跨订单累计。", { currency: monetary.currency, ratio: monetary.atg_per_unit })}</p></div>;
  } catch { return <p className="text-sm text-ink-mute">{t("填写有效金额和连续阶梯后显示试算结果。")}</p>; }
}
export function PurchaseTiersEditor({ value, disabled, change }: { value: string; disabled: boolean; change: (value: string) => void }) {
  const { t } = useI18n();
  const monetary = useMonetary();
  const [amount, setAmount] = useState("2000");
  let tiers: PurchaseTier[];
  try { tiers = JSON.parse(value); if (!Array.isArray(tiers)) throw new Error(); } catch { tiers = [{ from_money: "0", gift_percent: "0" }]; }
  const update = (i: number, key: keyof PurchaseTier, next: string) => change(JSON.stringify(tiers.map((tier, index) => index === i ? { ...tier, [key]: next } : tier)));
  return <div className="space-y-4"><div className="flex items-center gap-2"><h4 className="text-sm font-semibold">{t("阶梯赠送")}</h4><FieldHelp title={t("阶梯赠送")}>{t("每 1 {currency} 购买 {ratio} ATG。赠送只计算落在本阶区间的金额，填 1 表示 1%，最后一阶没有上限。", { currency: monetary.currency, ratio: monetary.atg_per_unit })}</FieldHelp></div><div className="space-y-3">{tiers.map((tier, i) => <div key={i} className="flex flex-wrap items-end gap-3 rounded-control border border-hairline p-3"><label className="min-w-36 flex-1 text-sm">{t("区间起点（结算币种）")}<Input className="mt-2" inputMode="decimal" value={tier.from_money} disabled={disabled || i === 0} onChange={e => update(i, "from_money", e.target.value)} /></label><label className="min-w-36 flex-1 text-sm">{t("本段赠送 %")}<Input className="mt-2" inputMode="decimal" value={tier.gift_percent} disabled={disabled} onChange={e => update(i, "gift_percent", e.target.value)} /></label>{i > 0 && <Button variant="ghost" type="button" disabled={disabled} onClick={() => change(JSON.stringify(tiers.filter((_, index) => index !== i)))}>{t("删除")}</Button>}</div>)}</div><Button variant="outline" type="button" disabled={disabled || tiers.length >= 100} onClick={() => { let next = ""; try { next = decimal(atoms(tiers.at(-1)?.from_money || "0") + 1000n * scale); } catch { /* Ask for a valid next threshold. */ } change(JSON.stringify([...tiers, { from_money: next, gift_percent: "0" }])); }}>{t("添加赠送阶梯")}</Button><label className="block text-sm">{t("试算采购金额（结算币种）")}<Input className="mt-2 max-w-64" inputMode="decimal" value={amount} onChange={e => setAmount(e.target.value)} /></label><PurchasePreview tiers={tiers} amount={amount} /></div>;
}
