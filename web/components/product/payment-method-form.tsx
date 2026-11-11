"use client";

import { useEffect, useState } from "react";
import { z } from "zod";
import { compareDecimal } from "@/lib/decimal";
import { useNavigationBlock } from "@/lib/navigation-guard";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { CommandResultSchema, Decimal, PaymentMethodCatalogSchema, type Entity, type Operation } from "@/lib/contracts";
import { ErrorPanel, Notice, useCommand } from "./shared";
import { useMonetary } from "./monetary";

type Catalog = z.infer<typeof PaymentMethodCatalogSchema>;
export function PaymentMethodForm({ operation, row, operatorID, catalog, onBlockedChange }: { operation: Operation; row?: Entity; operatorID: string; catalog: Catalog; onBlockedChange: (value: boolean) => void }) {
  const { t } = useI18n(), monetary = useMonetary();
  const [kind, setKind] = useState(String(row?.values.method_kind || ""));
  const [driver, setDriver] = useState(String(row?.values.driver || (row?.values.method_kind === "offline" ? "manual" : "")));
  const [enabled, setEnabled] = useState(row?.values.enabled !== false);
  const [collection, setCollection] = useState(row?.values.collection === true);
  const [payout, setPayout] = useState(row?.values.payout_enabled === true);
  const [mode, setMode] = useState(String(row?.values.fee_mode || "none"));
  const [fixed, setFixed] = useState(String(row?.values.flat_fee_money || "0"));
  const [percent, setPercent] = useState(String(row?.values.fee_percent || "0"));
  const [credentials, setCredentials] = useState<Record<string, string>>((row?.values.credentials as Record<string, string>) || { currency: monetary.currency });
  const [validation, setValidation] = useState("");
  const command = useCommand(operation.path.replace("{id}", encodeURIComponent(row?.id || "")), CommandResultSchema);
  const navigationBlocked = useNavigationBlock(command.busy || command.unresolved);
  const locked = command.busy || command.unresolved || !!command.result;
  useEffect(() => { onBlockedChange(command.busy || command.unresolved); return () => onBlockedChange(false); }, [command.busy, command.unresolved, onBlockedChange]);
  const selected = catalog.find(item => item.kind === kind);
  const onlineDrivers = selected?.drivers.filter(item => item.value !== "manual") || [];
  const effectiveDriver = selected?.drivers.some(item => item.value === driver) ? driver : onlineDrivers[0]?.value || "manual";
  const connection = selected?.drivers.find(item => item.value === effectiveDriver);
  function choose(value: string) {
    const next = catalog.find(item => item.kind === value);
    setKind(value); setDriver(next?.drivers.find(item => item.value !== "manual")?.value || next?.drivers[0]?.value || ""); setCollection(value === "offline" || !next?.payout); setPayout(false);
    setCredentials({ currency: monetary.currency }); setValidation("");
  }
  async function submit(event: React.FormEvent) {
    event.preventDefault(); if (!selected || command.busy) return;
    if (enabled && !collection && !(selected.payout && payout)) { setValidation("请选择至少一项用途，或停用此支付方式。"); return; }
    if (!Decimal.safeParse(fixed).success || !Decimal.safeParse(percent).success || mode === "percent" && compareDecimal(percent, "100") >= 0) { setValidation("请检查支付方式和手续费规则。"); return; }
    setValidation("");
    await command.run({ action: "open_method", method_kind: kind, enabled, collection, payout_enabled: selected.payout && payout, driver: collection ? effectiveDriver : "", credentials, fee_mode: mode, flat_fee_money: fixed, fee_percent: percent, expected_version: row?.version || 0 });
  }
  const fieldClass = "mt-2 h-11 w-full rounded-control border border-hairline bg-canvas-raised px-3";
  return <form onSubmit={submit} className="space-y-6">
    <p className="text-sm leading-7 text-ink-secondary">{t(kind === "offline" ? "线下支付是系统内置能力。启用所需业务并设置费用，双方实际收付款后登记完成。" : "先选择要办理的业务。人工提现无需接入接口；线上收款按通道要求配置。")}</p>
    <fieldset disabled={locked} className="space-y-5">
      <label className="block text-sm">{t("支付方式")}<select aria-label={t("支付方式")} required value={kind} disabled={!!row} onChange={e => choose(e.target.value)} className={fieldClass}><option value="">{t("请选择")}</option>{catalog.map(item => <option disabled={!!item.existing_id && !row} key={item.kind} value={item.kind}>{t(item.label)}</option>)}</select></label>
      {selected && <>
        <label className="flex items-center gap-3 text-sm"><input type="checkbox" checked={enabled} onChange={e => setEnabled(e.target.checked)} />{t("启用支付方式")}</label>
        <div className="space-y-3 rounded-control border border-hairline p-4"><p className="font-medium text-sm">{t("适用业务")}</p>
          <label className="flex items-center gap-3 text-sm"><input type="checkbox" checked={collection} onChange={e => setCollection(e.target.checked)} />{t("用于收款")}</label>
          {selected.payout ? <label className="flex items-center gap-3 text-sm"><input type="checkbox" checked={payout} onChange={e => setPayout(e.target.checked)} />{t("用于余额和佣金提现")}</label> : <p className="text-xs text-ink-mute">{t("此方式仅用于收款。")}</p>}
        </div>
        {collection && effectiveDriver !== "manual" && <section className="space-y-4"><h3 className="font-medium text-sm">{t("线上收款接入")}</h3>
          {onlineDrivers.length > 1 && <label className="block text-sm">{t("收款办理方式")}<select aria-label={t("收款办理方式")} value={effectiveDriver} onChange={e => { setDriver(e.target.value); setCredentials({ currency: monetary.currency }); }} className={fieldClass}>{onlineDrivers.map(item => <option key={item.value} value={item.value}>{t(item.label)}</option>)}</select></label>}
            <p className="text-xs leading-6 text-ink-secondary">{t("资料未齐可先保存，收款尚不开放，不影响已启用的人工提现。")}</p>
            {connection?.fields.map(field => <label key={field.name} className="block text-sm">{t(field.label)}<Input className="mt-2" type={field.secret ? "password" : "text"} autoComplete="off" value={credentials[field.name] || ""} placeholder={field.secret && row ? t("留空保留已保存的密钥") : undefined} onChange={e => setCredentials(previous => ({ ...previous, [field.name]: e.target.value }))} /></label>)}
        </section>}
        <section className="space-y-4 border-t border-hairline pt-5"><h3 className="font-medium text-sm">{t("手续费规则")}</h3>
          <label className="block text-sm">{t("手续费规则")}<select aria-label={t("手续费规则")} value={mode} onChange={e => setMode(e.target.value)} className={fieldClass}><option value="none">{t("不收费")}</option><option value="fixed">{t("固定金额")}</option><option value="percent">{t("按比例")}</option></select></label>
          {mode === "fixed" && <label className="block text-sm">{t("固定手续费")} {monetary.currency}<Input className="mt-2" inputMode="decimal" value={fixed} onChange={e => setFixed(e.target.value)} /></label>}
          {mode === "percent" && <label className="block text-sm">{t("手续费率 %")}<Input className="mt-2" inputMode="decimal" value={percent} onChange={e => setPercent(e.target.value)} /></label>}
          <p className="text-xs leading-6 text-ink-secondary">{t("收款在本金之外加收，提现从申请总额扣除。内部余额和佣金抵扣不收费；修改不影响已确认的交易。")}</p>
        </section>
      </>}
    </fieldset>
    {validation && <p role="alert" className="text-sm text-danger">{t(validation)}</p>}
    {!!command.error && <ErrorPanel error={command.error} />}
    {(command.unresolved || navigationBlocked) && <Notice>{t("请保持此办理面板，查询业务记录或重试本次操作。重试会复用原提交内容与同一操作编号，避免重复办理。")}</Notice>}
    {command.result ? <p role="status" className="text-sm text-success">{t("支付方式和手续费规则已保存。请按卡片状态核对可用业务。")}</p> : <Button disabled={!selected || command.busy} type="submit">{command.busy ? t("正在提交…") : command.unresolved ? t("重试本次操作") : t("保存支付方式")}</Button>}
  </form>;
}
