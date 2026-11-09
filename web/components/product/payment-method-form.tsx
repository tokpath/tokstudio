"use client";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { z } from "zod";
import { request } from "@/lib/api-client";
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
  const [asset, setAsset] = useState(String(row?.values.asset_network_version_id || ""));
  const [payoutCurrency, setPayoutCurrency] = useState(String(row?.values.payout_currency || monetary.currency));
  const [payoutRate, setPayoutRate] = useState(String(row?.values.payout_rate || ""));
  const [payoutNote, setPayoutNote] = useState(String(row?.values.payout_note || ""));
  const [networks, setNetworks] = useState<string[]>((row?.values.payout_networks as string[]) || []);
  const [validation, setValidation] = useState("");
  const command = useCommand(operation.path.replace("{id}", encodeURIComponent(row?.id || "")), CommandResultSchema);
  const navigationBlocked = useNavigationBlock(command.busy || command.unresolved);
  const locked = command.busy || command.unresolved || !!command.result;
  useEffect(() => { onBlockedChange(command.busy || command.unresolved); return () => onBlockedChange(false); }, [command.busy, command.unresolved, onBlockedChange]);
  const selected = catalog.find(item => item.kind === kind), connection = selected?.drivers.find(item => item.value === driver);
  const needsAsset = ["nowpayments", "evm_wallet"].includes(driver);
  const assets = useQuery({ queryKey: ["method-assets", operatorID, driver, kind], enabled: collection && needsAsset, queryFn: ({ signal }) => request(`/api/v1/operators/${encodeURIComponent(operatorID)}/references/payment-assets?driver=${encodeURIComponent(driver)}&method_kind=${encodeURIComponent(kind)}`, z.object({ items: z.array(z.object({ value: z.string(), label: z.string() })) }), { signal }) });
  const currencyOptions = Array.from(new Set([monetary.currency, payoutCurrency, "CNY", "USD", "EUR", "JPY", "RUB", "GBP", "HKD", "SGD", "AUD", "CAD", ...(kind === "usdt" ? ["USDT"] : [])].filter(Boolean)));
  function choose(value: string) {
    const next = catalog.find(item => item.kind === value);
    setKind(value); setDriver(next?.drivers[0]?.value || ""); setCollection(!next?.payout); setPayout(false);
    setCredentials({ currency: monetary.currency }); setAsset(""); setNetworks([]); setValidation("");
    const currency = value === "usdt" ? "USDT" : ["alipay", "wechat"].includes(value) ? "CNY" : monetary.currency;
    setPayoutCurrency(currency); setPayoutRate(currency === monetary.currency ? "1" : ""); setPayoutNote("");
  }
  async function submit(event: React.FormEvent) {
    event.preventDefault(); if (!selected || command.busy) return;
    if (enabled && !collection && !(selected.payout && payout)) { setValidation("请选择至少一项用途，或停用此支付方式。"); return; }
    if (!Decimal.safeParse(fixed).success || !Decimal.safeParse(percent).success || mode === "percent" && compareDecimal(percent, "100") >= 0) { setValidation("请检查支付方式和手续费规则。"); return; }
    if (payout && payoutCurrency !== monetary.currency && (!Decimal.safeParse(payoutRate).success || compareDecimal(payoutRate, "0") <= 0)) { setValidation("请填写有效的提现兑换比例。"); return; }
    if (payout && kind === "usdt" && !networks.length) { setValidation("请选择经营方能够办理的提现网络。"); return; }
    setValidation("");
    await command.run({ action: "open_method", method_kind: kind, enabled, collection, payout_enabled: selected.payout && payout, driver: collection ? driver : "", asset_network_version_id: asset, credentials, fee_mode: mode, flat_fee_money: fixed, fee_percent: percent, payout_currency: payoutCurrency, payout_rate: payoutCurrency === monetary.currency ? "1" : payoutRate || "0", payout_note: payoutNote, payout_networks: networks, expected_version: row?.version || 0 });
  }
  const fieldClass = "mt-2 h-11 w-full rounded-control border border-hairline bg-canvas-raised px-3";
  return <form onSubmit={submit} className="space-y-6">
    <p className="text-sm leading-7 text-ink-secondary">{t(kind === "offline" ? "线下支付是系统内置能力。启用所需业务并设置费用，双方实际收付款后登记完成。" : "先选择要办理的业务。人工提现无需接入接口；线上收款按通道要求配置。")}</p>
    <fieldset disabled={locked} className="space-y-5">
      {kind === "offline" ? <h3 className="font-medium">{t("线下支付")}</h3> : <label className="block text-sm">{t("支付方式")}<select aria-label={t("支付方式")} required value={kind} disabled={!!row} onChange={e => choose(e.target.value)} className={fieldClass}><option value="">{t("请选择")}</option>{catalog.filter(item => item.kind !== "offline").map(item => <option disabled={!!item.existing_id && !row} key={item.kind} value={item.kind}>{t(item.label)}</option>)}</select></label>}
      {selected && <>
        <label className="flex items-center gap-3 text-sm"><input type="checkbox" checked={enabled} onChange={e => setEnabled(e.target.checked)} />{t("启用支付方式")}</label>
        <div className="space-y-3 rounded-control border border-hairline p-4"><p className="font-medium text-sm">{t("适用业务")}</p>
          <label className="flex items-center gap-3 text-sm"><input type="checkbox" checked={collection} onChange={e => setCollection(e.target.checked)} />{t("用于收款")}</label>
          {selected.payout ? <label className="flex items-center gap-3 text-sm"><input type="checkbox" checked={payout} onChange={e => setPayout(e.target.checked)} />{t("用于余额和佣金提现")}</label> : <p className="text-xs text-ink-mute">{t("此方式仅用于收款。")}</p>}
        </div>
        {collection && <section className="space-y-4"><h3 className="font-medium text-sm">{t(driver === "manual" ? "人工收款说明" : "线上收款接入")}</h3>
          {selected.drivers.length > 1 && <label className="block text-sm">{t("收款办理方式")}<select aria-label={t("收款办理方式")} value={driver} onChange={e => { setDriver(e.target.value); setCredentials({ currency: monetary.currency }); setAsset(""); }} className={fieldClass}>{selected.drivers.map(item => <option key={item.value} value={item.value}>{t(item.label)}</option>)}</select></label>}
          {driver === "manual" ? <>
            <p className="text-xs leading-6 text-ink-secondary">{t("双方线下完成收付款后登记实际收款事实，系统按本金入账和交付。")}</p>
            <p className="text-xs">{t("付款币种")} · {["alipay", "wechat"].includes(kind) ? "CNY" : kind === "usdt" ? "USDT" : credentials.currency || monetary.currency}</p>
            <label className="block text-sm">{t("收款信息（可选）")}<Input className="mt-2" value={credentials.recipient_reference || ""} onChange={e => setCredentials(previous => ({ ...previous, recipient_reference: e.target.value }))} /></label>
            <label className="block text-sm">{t("付款说明（可选）")}<Input className="mt-2" value={credentials.instructions || ""} onChange={e => setCredentials(previous => ({ ...previous, instructions: e.target.value }))} /></label>
          </> : <>
            <p className="text-xs leading-6 text-ink-secondary">{t("资料未齐可先保存，收款尚不开放，不影响已启用的人工提现。")}</p>
            {needsAsset && <label className="block text-sm">{t("收款资产与网络")}<select aria-label={t("收款资产与网络")} value={asset} onChange={e => setAsset(e.target.value)} className={fieldClass}><option value="">{t("请选择")}</option>{assets.data?.items.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}</select>{assets.data?.items.length === 0 && <p className="mt-2 text-xs">{t("等待平台配置可用的资产与网络。")}</p>}</label>}
            {connection?.fields.map(field => <label key={field.name} className="block text-sm">{t(field.label)}<Input className="mt-2" type={field.secret ? "password" : "text"} autoComplete="off" value={credentials[field.name] || ""} placeholder={field.secret && row ? t("留空保留已保存的密钥") : undefined} onChange={e => setCredentials(previous => ({ ...previous, [field.name]: e.target.value }))} /></label>)}
          </>}
        </section>}
        {selected.payout && payout && <section className="space-y-4 border-t border-hairline pt-5"><h3 className="font-medium text-sm">{t("人工提现")}</h3>
          <p className="text-xs leading-6 text-ink-secondary">{t("经营方审批后自行付款，再登记完成。余额和佣金提现共用以下资料和费用规则。")}</p>
          <div className="rounded-control bg-canvas p-4"><p className="text-sm font-medium">{t("客户填写的收款资料")}</p><p className="mt-2 text-xs leading-6">{t(selected.payout_summary || "")}</p><ul className="mt-2 space-y-1 text-sm">{selected.payout_fields?.map(field => <li key={field.name}>{t(field.label)} · {t(field.optional ? "可选" : "必填")}</li>)}</ul></div>
          {kind === "usdt" && <fieldset className="space-y-2"><legend className="text-sm">{t("可办理的提现网络")}</legend>{["TRC20", "ERC20"].map(network => <label key={network} className="flex items-center gap-3 text-sm"><input type="checkbox" checked={networks.includes(network)} onChange={e => setNetworks(previous => e.target.checked ? [...previous, network] : previous.filter(item => item !== network))} />{network}</label>)}</fieldset>}
          <label className="block text-sm">{t("提现到账币种")}<select aria-label={t("提现到账币种")} disabled={["usdt", "alipay", "wechat"].includes(kind)} value={payoutCurrency} onChange={e => { setPayoutCurrency(e.target.value); setPayoutRate(e.target.value === monetary.currency ? "1" : ""); }} className={fieldClass}>{currencyOptions.map(code => <option key={code} value={code}>{code}</option>)}</select></label>
          {payoutCurrency !== monetary.currency && <><label className="block text-sm">{t("提现兑换比例")} · 1 {monetary.currency} = <Input aria-label={t("提现兑换比例")} className="mt-2" inputMode="decimal" value={payoutRate} onChange={e => setPayoutRate(e.target.value)} /> {payoutCurrency}</label><p className="text-xs leading-6">{t("这是本方式承诺的实际付款比例。客户申请前看到到账金额，申请后固定；调整只影响新申请。")}</p></>}
          <label className="block text-sm">{t("提现办理说明（可选）")}<textarea className="mt-2 min-h-24 w-full rounded-control border border-hairline bg-canvas-raised p-3" value={payoutNote} onChange={e => setPayoutNote(e.target.value)} /></label>
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
