"use client";

import { useState, type FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { useI18n } from "@/components/i18n";
import { request } from "@/lib/api-client";
import { CommandResultSchema, Decimal, MonetarySchema } from "@/lib/contracts";
import { compareDecimal } from "@/lib/decimal";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { useMonetary, monetaryRefetchInterval } from "./monetary";
import { AlertText, ErrorPanel, Notice, When, useCommand } from "./shared";

type Draft = { currency: string; atg_per_unit: string; effective_at: string; expected_version: number; confirmed: true };
export function MonetarySettings({ canManage }: { canManage: boolean }) {
  const { t } = useI18n();
  const current = useMonetary();
  const settings = useQuery({ queryKey: ["monetary-settings"], queryFn: () => request("/api/v1/monetary/settings", MonetarySchema), enabled: canManage, refetchInterval: query => monetaryRefetchInterval(query.state.data) });
  const monetary = settings.data && !(current.status === "locked" && current.version >= settings.data.version) ? settings.data : current;
  const save = useCommand("/api/v1/monetary/settings", CommandResultSchema);
  const cancel = useCommand("/api/v1/monetary/cancel", CommandResultSchema);
  const [draft, setDraft] = useState<Draft>();
  const [error, setError] = useState<string>();
  const [accepted, setAccepted] = useState(false);
  const locked = monetary.status === "locked";
  const busy = save.busy || cancel.busy || save.unresolved || cancel.unresolved;
  function review(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget), currency = String(form.get("currency") || "").trim().toUpperCase(), ratio = String(form.get("atg_per_unit") || "").trim(), at = new Date(String(form.get("effective_at") || ""));
    if (!/^[A-Z]{3}$/.test(currency) || !Decimal.safeParse(ratio).success || compareDecimal(ratio, "0") <= 0 || Number.isNaN(at.getTime()) || at.getTime() <= Date.now()) { setError("请填写有效的三位 ISO 币种、正数 ATG 比例和未来生效时间。"); return; }
    setError(undefined); setAccepted(false); setDraft({ currency, atg_per_unit: ratio, effective_at: at.toISOString(), expected_version: monetary.version, confirmed: true });
  }
  return <Card>
    <h2 className="font-semibold">{t("全系统结算币种与 ATG 基础比例")}</h2>
    <p className="mt-2 text-sm leading-7 text-ink-secondary">{monetary.status === "unconfigured" ? t("营业前请选择全系统的结算币种和 ATG 基础比例。") : t(monetary.status === "scheduled" ? "预约基础规则：每 1 {currency} 对应 {ratio} ATG，将于预约时间生效。" : "全系统使用同一结算币种。每 1 {currency} 对应 {ratio} ATG。", { currency: monetary.currency, ratio: monetary.atg_per_unit })}</p>
    {!locked && <p className="mt-2 text-xs leading-6 text-ink-mute">{t("生效前全系统资金与模型调用业务尚未开放。生效时间即基础规则正式启用的时间，无需另外开业。")}</p>}
    <p className="mt-2 text-xs leading-6 text-ink-mute">{t(locked ? "已生效并永久锁定。暂停营业不会解锁。" : monetary.status === "scheduled" ? "已预约生效；生效前可取消并重新设置。" : "尚未设置，营业前须由平台负责人完成。")}{monetary.effective_at && <> <When value={monetary.effective_at} /></>}</p>
    {canManage && settings.isError && <div className="mt-4"><ErrorPanel error={settings.error} retry={() => void settings.refetch()} /></div>}
    {canManage && !locked && monetary.status === "unconfigured" && !draft && <form className="mt-5 space-y-4" onSubmit={review}>
      <fieldset className="grid gap-4 sm:grid-cols-3" disabled={busy || settings.isPending}>
        <label className="text-sm">{t("结算币种（ISO 代码）")}<Input className="mt-2 uppercase" name="currency" defaultValue={monetary.currency} minLength={3} maxLength={3} pattern="[A-Za-z]{3}" required /></label>
        <label className="text-sm">{t("每 1 单位结算币种购买的 ATG")}<Input className="mt-2" name="atg_per_unit" inputMode="decimal" defaultValue={monetary.atg_per_unit} required /></label>
        <label className="text-sm">{t("生效时间")}<Input className="mt-2" name="effective_at" type="datetime-local" required /></label>
      </fieldset>
      <p className="text-xs leading-6 text-ink-mute">{t("时间按当前设备时区 {timezone} 输入，保存为统一时刻。", { timezone: Intl.DateTimeFormat().resolvedOptions().timeZone })}</p>
      <Button disabled={busy || settings.isPending}>{t("核对并查看风险说明")}</Button>
    </form>}
    {canManage && draft && !locked && <div className="mt-5 space-y-4">
      <Notice><p>{t("请再次确认：{currency}，每 1 单位购买 {ratio} ATG，生效于：", { currency: draft.currency, ratio: draft.atg_per_unit })} <When value={draft.effective_at} /></p><p>{t("生效后币种和基础比例永久锁定，暂停营业也无法修改。该比例只用于购买 ATG 换算和采购阶梯赠送；套餐等明确配置的 ATG 数量不会自动改变。系统不提供手工或自动汇率，不汇总跨币种利润。")}</p></Notice>
      <Notice>{t("请核对已录入的套餐、报价和费率：其中金额将按本次选定币种解释，ATG 数量保持原值；系统不会按外汇汇率换算已有配置。生效前资金与模型调用业务尚未开放，生效后规则永久锁定。")}</Notice>
      <label className="flex items-start gap-2 text-sm leading-6"><input type="checkbox" checked={accepted} disabled={busy} onChange={event => setAccepted(event.target.checked)} />{t("我已核对并理解永久锁定及换算规则")}</label>
      <div className="flex flex-wrap gap-3"><Button disabled={save.busy || (!save.unresolved && !accepted)} onClick={async () => { if (await save.run(save.unresolved ? undefined : draft)) { setDraft(undefined); setAccepted(false); } }}>{t(save.unresolved ? "重试原操作" : "确认预约生效")}</Button><Button variant="outline" disabled={busy} onClick={() => setDraft(undefined)}>{t("返回修改")}</Button></div>
    </div>}
    {canManage && monetary.status === "scheduled" && !draft && <Button className="mt-5" variant="outline" disabled={cancel.busy || save.unresolved} onClick={() => void cancel.run(cancel.unresolved ? undefined : { expected_version: monetary.version })}>{t(cancel.unresolved ? "重试原操作" : "取消预约并重新设置")}</Button>}
    {!!error && <AlertText className="mt-4 text-sm text-danger">{t(error)}</AlertText>}
    {!!save.error && <div className="mt-4"><ErrorPanel error={save.error} /></div>}
    {!!cancel.error && <div className="mt-4"><ErrorPanel error={cancel.error} /></div>}
  </Card>;
}
