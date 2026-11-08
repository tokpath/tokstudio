"use client";
import { useProduct } from "./context";
import { useEffect, useState, type FormEvent } from "react";
import Link from "next/link";
import { useQueryClient } from "@tanstack/react-query";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { CommandResultSchema, Decimal, type Entity, type ResourcePage } from "@/lib/contracts";
import { compareDecimal } from "@/lib/decimal";
import { useNavigationBlock } from "@/lib/navigation-guard";
import { ErrorPanel, Heading, Money, Notice, Status, useCommand, When } from "./shared";

const descriptions: Record<string, [string, string]> = {
  sales_space_atg: ["剩余销售空间ATG", "不足时，继续卖可能超出履约能力，需要补货或限制新增销售。"],
  procurement_available_atg: ["剩余采购余额ATG", "不足时，实际调用可能停服，需要补货。"],
};
export function RiskSettings({ operatorID, page }: { operatorID: string; page: ResourcePage }) {
  const { t } = useI18n();
  const cache = useQueryClient();
  const {session}=useProduct();
  const stopped=session?.kind==="staff"&&!!session.workspaces.find(w=>w.operator.id===operatorID)?.transition;
  if(stopped)return <><Heading title={t("风险预警")} description={t("渠道已进入接管流程，预警配置仅供查询。")} /><div className="space-y-4">{page.items.map(row=><Card key={row.id}><h2 className="font-semibold">{t(String(row.values.metric)==="sales_space_atg"?"剩余销售空间ATG":"剩余采购余额ATG")}</h2><p className="mt-3">{t("提醒阈值（ATG）")}：{typeof row.values.warning==="string"?<Money value={row.values.warning} unit="ATG"/>:t("尚未核实")}</p></Card>)}</div></>;
  return <><Heading title={t("风险预警")} description={t("两项补货预警默认已启用，可直接调整。仅提醒本经营方，不会自动补货或限制销售。")} actions={<Button variant="outline" onClick={() => void cache.invalidateQueries({ queryKey: ["resource"] })}>{t("刷新")}</Button>} />
    <div className="grid items-start gap-6 min-[1200px]:grid-cols-2">{page.items.filter(row => descriptions[String(row.values.metric)]).map(row => <RiskCard key={row.id} operatorID={operatorID} row={row} />)}</div>
  </>;
}

function RiskCard({ operatorID, row }: { operatorID: string; row: Entity }) {
  const { t } = useI18n();
  const cache = useQueryClient();
  const metric = String(row.values.metric), [title, description] = descriptions[metric];
  const savedRecipients = Array.isArray(row.values.recipients) ? row.values.recipients.map(String) : [];
  const [warning, setWarning] = useState(String(row.values.warning));
  const [minutes, setMinutes] = useState(String(row.values.reminder_minutes));
  const [enabled, setEnabled] = useState(row.values.enabled === true);
  const [recipients, setRecipients] = useState(savedRecipients);
  const [dirty, setDirty] = useState(false), [validation, setValidation] = useState("");
  const [version, setVersion] = useState(row.version);
  const command = useCommand(`/api/v1/operators/${encodeURIComponent(operatorID)}/alerts`, CommandResultSchema);
  const frozen = command.busy || command.unresolved;
  useNavigationBlock(frozen);
  useEffect(() => {
    if (dirty || frozen) return;
    setWarning(String(row.values.warning)); setMinutes(String(row.values.reminder_minutes));
    setEnabled(row.values.enabled === true); setRecipients(Array.isArray(row.values.recipients) ? row.values.recipients.map(String) : []); setVersion(row.version);
  }, [row, dirty, frozen]);
  const choices = (Array.isArray(row.values.staff_choices) ? row.values.staff_choices : []).flatMap(value => value && typeof value === "object" && !Array.isArray(value) && typeof value.value === "string" && typeof value.label === "string" ? [{ value: value.value, label: value.label }] : []);
  const owners = Array.isArray(row.values.default_owner_ids) ? row.values.default_owner_ids.map(String) : [];
  const change = (fn: () => void) => { if (frozen) return; fn(); setDirty(true); setValidation(""); command.reset(); };
  async function save(event: FormEvent) {
    event.preventDefault();
    if (command.unresolved) { const result = await command.run(undefined); if (result) setDirty(false); return; }
    if (!Decimal.safeParse(warning).success || compareDecimal(warning, "0") <= 0) { setValidation("提醒阈值必须大于 0 ATG。"); return; }
    if (!Decimal.safeParse(minutes).success || compareDecimal(minutes, "1") < 0 || compareDecimal(minutes, "43200") > 0) { setValidation("提醒间隔须为 1 至 43200 分钟。"); return; }
    const result = await command.run({ metric, warning, recipients, reminder_minutes: minutes, enabled, expected_version: version });
    if (result) setDirty(false);
  }
  return <Card><section aria-label={t(title)} className="space-y-5">
    <div className="flex flex-wrap items-start justify-between gap-3"><h2 className="text-lg font-semibold">{t(title)}</h2><Status value={row.status || "pending_configuration"} /></div>
    <p className="text-sm leading-7 text-ink-secondary">{t(description)}</p>
    <div><p className="text-xs text-ink-mute">{t("当前值")}</p><p className="mt-2 text-3xl font-semibold">{typeof row.values.value === "string" ? <Money value={row.values.value} /> : t("尚未核实")}</p></div>
    {row.values.value == null && <Notice>{t("等待平台配置可用模型及供货价格，暂时无法核实当前值；预警设置仍可保存。")}</Notice>}
    <form onSubmit={event => void save(event)} className="space-y-5 border-t border-hairline pt-5">
      <fieldset disabled={frozen} className="space-y-5">
        <label className="flex min-h-11 items-center gap-3 text-sm"><input type="checkbox" checked={enabled} onChange={event => change(() => setEnabled(event.target.checked))} />{t("启用预警")}</label>
        <label className="block text-sm font-medium">{t("提醒阈值（ATG）")}<Input className="mt-2 block max-w-xs" inputMode="decimal" value={warning} onChange={event => change(() => setWarning(event.target.value))} /></label>
        <p className="text-xs leading-6 text-ink-mute">{t("达到或低于提醒阈值时通知，达到或低于 0 时显示严重。初始阈值为 100 ATG，请按采购规模和补货时间调整。")}</p>
        <details className="rounded-control border border-hairline p-4"><summary className="cursor-pointer text-sm font-medium">{t("通知设置")}</summary><div className="mt-4 space-y-4">
          <p className="text-xs leading-6 text-ink-secondary">{t("默认通知本经营方在岗负责人；也可指定通知人员。站内通知始终保留，邮箱已验证时同时发送邮件。")}</p>
          <fieldset className="space-y-2"><legend className="mb-2 text-sm">{t("通知人员")}</legend>
            <label className="flex min-h-11 items-center gap-3 text-sm"><input type="checkbox" checked={recipients.length === 0} onChange={() => change(() => setRecipients([]))} />{t("跟随本经营方负责人")}</label>
            {choices.map(choice => <label key={choice.value} className="flex min-h-11 items-center gap-3 text-sm"><input type="checkbox" checked={recipients.includes(choice.value)} onChange={event => change(() => setRecipients(event.target.checked ? [...recipients, choice.value] : recipients.filter(id => id !== choice.value)))} />{choice.label}</label>)}
          </fieldset>
          {!recipients.length && !owners.length && <Notice>{t("尚未开通在岗负责人；配置已启用，负责人开通后自动接收通知。")}</Notice>}
          <label className="block text-sm">{t("持续风险提醒间隔（分钟）")}<Input className="mt-2 block max-w-xs" inputMode="decimal" value={minutes} onChange={event => change(() => setMinutes(event.target.value))} /></label>
          <p className="text-xs leading-6 text-ink-mute">{t("默认每 60 分钟提醒一次；恢复正常时通知一次。")}</p>
        </div></details>
      </fieldset>
      {validation && <p role="alert" className="text-sm text-danger">{t(validation)}</p>}
      {!!command.error && <ErrorPanel error={command.error} />}
      {command.unresolved && <Notice>{t("保存结果待核实，请重试原设置；核实前不能修改内容。")}</Notice>}
      {command.result && !dirty && <p role="status" className="text-sm text-success">{t("预警设置已保存。")}</p>}
      <div className="flex flex-wrap items-center gap-3"><Button type="submit" disabled={command.busy || !dirty && !command.unresolved}>{t(command.busy ? "正在提交，请稍候…" : command.unresolved ? "重试原设置" : "保存预警设置")}</Button>
        {dirty && !frozen && <Button type="button" variant="ghost" onClick={() => { setDirty(false); setValidation(""); command.reset(); void cache.invalidateQueries({ queryKey: ["resource"] }); }}>{t("放弃修改")}</Button>}
        <Button asChild variant="outline"><Link href={`/manage/${encodeURIComponent(operatorID)}/procurement`}>{t("前往采购补货")}</Link></Button>
      </div>
    </form>
    <p className="text-xs text-ink-mute">{t("最近通知")}：{typeof row.values.last_notified_at === "string" ? <When value={row.values.last_notified_at} /> : t("尚未发送")}</p>
  </section></Card>;
}
