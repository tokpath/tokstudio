"use client";

import { useState } from "react";
import { CommandResultSchema, type Entity, type Operation } from "@/lib/contracts";
import { previewWithdrawal } from "@/lib/withdrawal-fee";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ErrorPanel, Money, Notice, Status, useCommand } from "./shared";

export type PayoutField = { name: string; label: string; help: string; kind: "text" | "select"; options?: { value: string; label: string }[] };
export type PayoutMethod = { id: string; name: string; kind: string; kind_label: string; summary: string; flat_fee_money: string; fee_rate: string; minimum_money: string; fields: PayoutField[] };

const reasons: Record<string, string> = {
  invalid: "请输入提现金额。",
  too_small: "可支付金额为 0。不足最小支付单位的尾数仍留在佣金钱包。",
  below_minimum: "低于最低提现额度。",
  fee_exceeds: "手续费不低于可支付金额，无法申请。",
};

export function CustomerWithdrawalRequest({ methods, digits }: { methods: PayoutMethod[]; digits: number }) {
  const { t } = useI18n();
  const [amount, setAmount] = useState("");
  const [methodID, setMethodID] = useState(methods.find(method => method.kind)?.id || methods[0]?.id || "");
  const [recipient, setRecipient] = useState<Record<string, string>>({});
  const [notice, setNotice] = useState("");
  const command = useCommand("/api/v1/me/withdrawals", CommandResultSchema);
  const selected = methods.find(method => method.id === methodID);
  const preview = amount ? previewWithdrawal({ amount, flatFee: selected?.flat_fee_money || "0", feeRate: selected?.fee_rate || "0", minimum: selected?.minimum_money || "0", digits }) : undefined;
  const ready = !!selected?.kind && preview?.ok === true && selected.fields.every(field => (recipient[field.name] || "").trim());

  async function submit() {
    if (!selected || !preview || !preview.ok || command.busy) return;
    setNotice("");
    const body = { method_id: selected.id, amount_money: amount, recipient: Object.fromEntries(selected.fields.map(field => [field.name, (recipient[field.name] || "").trim()])) };
    const result = await command.run(body);
    if (result) { setRecipient({}); setNotice(result.message || "已提交提现申请。"); }
  }

  return <Card>
    <div className="mb-5"><h2 className="text-lg font-semibold">{t("申请提现")}</h2><p className="mt-2 text-sm leading-6 text-ink-secondary">{t("选择提现方式并填写金额后，可以立刻比较手续费。选定后填写该方式要求的收款资料，一次提交。")}</p></div>
    {!methods.length ? <Notice>{t("请等待经营方配置可用提现方式。")}</Notice> : <div className="space-y-5">
      <label className="block text-sm">{t("提现金额 结算币种")}<Input className="mt-2" inputMode="decimal" value={amount} onChange={event => { setAmount(event.target.value); setNotice(""); }} required />
        <p className="mt-2 text-xs leading-6 text-ink-mute">{t("不足最小支付单位的尾数仍留在佣金钱包，不计入本次申请。")}</p>
      </label>
      <fieldset className="space-y-3"><legend className="mb-3 text-sm">{t("提现方式")}</legend>{methods.map(method => {
        const quote = amount ? previewWithdrawal({ amount, flatFee: method.flat_fee_money, feeRate: method.fee_rate, minimum: method.minimum_money, digits }) : undefined;
        return <label key={method.id} className={`block cursor-pointer rounded-control border p-4 ${method.id === methodID ? "border-brand bg-brand-soft/20" : "border-hairline"}`}>
          <span className="flex flex-wrap items-start justify-between gap-3"><span className="flex items-start gap-2"><input type="radio" name="payout-method" checked={method.id === methodID} onChange={() => { setMethodID(method.id); setNotice(""); }} /><span><span className="font-medium">{method.name}</span><span className="mt-1 block text-xs text-ink-mute">{t(method.kind_label)}</span></span></span>
            {quote?.ok ? <span className="text-right text-sm"><span className="block text-ink-mute">{t("手续费")} <Money value={quote.fee} unit="MONEY" /></span><span className="mt-1 block font-medium">{t("实付")} <Money value={quote.net} unit="MONEY" /></span></span> : quote ? <span className="text-xs text-danger">{t(reasons[quote.reason])}</span> : <span className="text-xs text-ink-mute">{method.fee_rate !== "0" ? t("按比例收取手续费") : t("按固定金额收取手续费")}</span>}
          </span>
        </label>;
      })}</fieldset>
      {selected && !selected.kind && <Notice>{t("这个方式尚未指定收款材料。请联系经营方选定支付方式和收款材料后再申请。")}</Notice>}
      {selected?.kind && <div className="space-y-4"><div><h3 className="text-sm font-medium">{t("收款资料")}</h3><p className="mt-2 text-sm leading-6 text-ink-secondary">{t(selected.summary)}</p></div>{selected.fields.map(field => <div key={field.name} className="text-sm"><label className="block">{t(field.label)}{field.kind === "select" ? <select className="mt-2 h-11 w-full rounded-control border border-hairline bg-canvas-raised px-3" value={recipient[field.name] || ""} onChange={event => setRecipient(current => ({ ...current, [field.name]: event.target.value }))} required><option value="">{t("请选择")}</option>{field.options?.map(option => <option key={option.value} value={option.value}>{t(option.label)}</option>)}</select> : <Input className="mt-2" value={recipient[field.name] || ""} onChange={event => setRecipient(current => ({ ...current, [field.name]: event.target.value }))} required />}</label>{field.help && <p className="mt-2 text-xs leading-6 text-ink-mute">{t(field.help)}</p>}</div>)}
        {preview?.ok && <p className="text-sm leading-7">{t("本次申请总额")} <Money value={preview.amount} unit="MONEY" /> · {t("手续费")} <Money value={preview.fee} unit="MONEY" /> · {t("实付")} <Money value={preview.net} unit="MONEY" /></p>}
        {preview && !preview.ok && <p role="alert" className="text-sm text-danger">{t(reasons[preview.reason])}</p>}
      </div>}
      {!!command.error && <ErrorPanel error={command.error} />}
      {notice && <Notice>{t(notice)}</Notice>}
      <Button type="button" disabled={!ready || command.busy} onClick={() => void submit()}>{command.busy ? t("正在提交…") : t("提交提现申请")}</Button>
    </div>}
  </Card>;
}

export function CustomerWithdrawalApplications({ rows, cancel, onCancel }: { rows: Entity[]; cancel?: Operation; onCancel: (operation: Operation, row: Entity) => void }) {
  const { t } = useI18n();
  const applications = rows.filter(row => row.status !== "quoted");
  return <div className="mt-8 space-y-3"><h2 className="text-lg font-semibold">{t("提现申请")}</h2><p className="text-sm leading-6 text-ink-secondary">{t("尚未审核的申请可以取消。取消后释放这笔佣金占用。")}</p>{!applications.length ? <Notice>{t("还没有提现申请。")}</Notice> : applications.map(row => <Card key={row.id}><div className="flex flex-wrap items-start justify-between gap-3"><div><p className="font-medium">{String(row.values.method_name || t("提现申请"))}</p><p className="mt-2 whitespace-pre-wrap text-sm leading-6 text-ink-secondary">{String(row.values.recipient_reference || "")}</p></div>{row.status && <Status value={row.status} />}</div><div className="mt-4 grid gap-3 text-sm sm:grid-cols-3"><div><p className="text-xs text-ink-mute">{t("总额")}</p><Money value={String(row.values.amount_money || "0")} unit="MONEY" /></div><div><p className="text-xs text-ink-mute">{t("手续费")}</p><Money value={String(row.values.fee_money || "0")} unit="MONEY" /></div><div><p className="text-xs text-ink-mute">{t("实付")}</p><Money value={String(row.values.net_money || "0")} unit="MONEY" /></div></div>{cancel && row.allowed_actions?.includes("cancel") && <Button type="button" variant="outline" className="mt-4" onClick={() => onCancel(cancel, row)}>{t("取消申请")}</Button>}</Card>)}
  </div>;
}
