"use client";

import { payoutRecipientText } from "@/lib/payout-recipient";
import { useEffect, useState } from "react";
import { CommandResultSchema, type Entity, type Operation } from "@/lib/contracts";
import { useQueryClient } from "@tanstack/react-query";
import { APIError } from "@/lib/api-client";
import { useNavigationBlock } from "@/lib/navigation-guard";
import { previewWithdrawal, previewPayout } from "@/lib/withdrawal-fee";
import { useMonetary } from "./monetary";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ErrorPanel, Money, Notice, Status, useCommand, withdrawalStatusLabels } from "./shared";

export type PayoutField = { name: string; label: string; help: string; kind: "text" | "select"; optional?:boolean; options?: { value: string; label: string }[] };
export type PayoutMethod = { payout_currency?:string;payout_rate?:string;payout_digits?:number;payout_note?:string; id: string; policy_id: string; name: string; kind: string; kind_label: string; summary: string; flat_fee_money: string; fee_rate: string; minimum_money: string; fields: PayoutField[] };

const reasons: Record<string, string> = {
  invalid: "请输入提现金额。",
  below_minimum: "低于最低提现额度。",
  fee_exceeds: "手续费不低于可支付金额，无法申请。",
};
function payoutReason(reason: string, walletName: string) {
  if (reason === "too_small") return walletName === "支付余额" ? "可支付金额为 0。不足最小支付单位的尾数仍留在支付余额。" : "可支付金额为 0。不足最小支付单位的尾数仍留在佣金钱包。";
  return reasons[reason];
}
function receivedLabel(walletName: string) {
  return walletName === "支付余额" ? "实得" : "实付";
}

export function CustomerWithdrawalRequest({ methods, digits, submitPath = "/api/v1/me/withdrawals", walletName = "佣金钱包" }: { methods: PayoutMethod[]; digits: number; submitPath?: string; walletName?: string }) {
  const { t } = useI18n();
  const monetary = useMonetary();
  const [amount, setAmount] = useState("");
  const [methodID, setMethodID] = useState(methods[0]?.id || "");
  const [recipient, setRecipient] = useState<Record<string, string>>({});
  const [methodSnapshot, setMethodSnapshot] = useState<PayoutMethod[]>();
  const [notice, setNotice] = useState("");
  const command = useCommand(submitPath, CommandResultSchema);
  const cache = useQueryClient();
  useEffect(() => { if (command.error instanceof APIError && command.error.code === "withdrawal_terms_changed") void cache.invalidateQueries(); }, [cache, command.error]);
  const frozen = command.busy || command.unresolved;
  const blocked = useNavigationBlock(frozen);
  const displayedMethods = frozen && methodSnapshot ? methodSnapshot : methods;
  const selected = displayedMethods.find(method => method.id === methodID);
  const preview = amount ? previewWithdrawal({ amount, flatFee: selected?.flat_fee_money || "0", feeRate: selected?.fee_rate || "0", minimum: selected?.minimum_money || "0", digits }) : undefined;
  const payoutAmount=preview?.ok && selected ? previewPayout(preview.net,selected.payout_rate || "1",selected.payout_digits ?? digits) : undefined;
  const ready = !!selected && preview?.ok === true && !!payoutAmount && selected.fields.every(field => field.optional || (recipient[field.name] || "").trim());

  async function submit() {
    if (!selected || !preview || !preview.ok || command.busy) return;
    setNotice("");
    if (!command.unresolved) setMethodSnapshot(methods);
    const body = { method_id: selected.id, policy_id: selected.policy_id, amount_money: amount, recipient: Object.fromEntries(selected.fields.map(field => [field.name, (recipient[field.name] || "").trim()])) };
    const result = await command.run(body);
    if (result) { setRecipient({}); setNotice(result.message || "已提交提现申请。"); }
  }

  return <Card>
    <div className="mb-5"><h2 className="text-lg font-semibold">{t("申请提现")}</h2><p className="mt-2 text-sm leading-6 text-ink-secondary">{t("选择提现方式并填写金额后，可以立刻比较手续费。选定后填写该方式要求的收款资料，一次提交。")}</p></div>
    {!displayedMethods.length ? <Notice>{t("请等待经营方配置可用提现方式。")}</Notice> : <div className="space-y-5">
      <fieldset disabled={frozen} className="space-y-5">
      <label className="block text-sm">{t("提现金额 结算币种")}<Input className="mt-2" inputMode="decimal" value={amount} onChange={event => { setAmount(event.target.value); setNotice(""); }} required /></label>
      <p className="mt-2 text-xs leading-6 text-ink-mute">{t(walletName === "支付余额" ? "不足最小支付单位的尾数仍留在支付余额，不计入本次申请。" : "不足最小支付单位的尾数仍留在佣金钱包，不计入本次申请。")}</p>
      <fieldset className="space-y-3"><legend className="mb-3 text-sm">{t("提现方式")}</legend>{displayedMethods.map(method => {
        const quote = amount ? previewWithdrawal({ amount, flatFee: method.flat_fee_money, feeRate: method.fee_rate, minimum: method.minimum_money, digits }) : undefined;
        return <label key={method.id} className={`block cursor-pointer rounded-control border p-4 ${method.id === methodID ? "border-brand bg-brand-soft/20" : "border-hairline"}`}>
          <span className="flex flex-wrap items-start justify-between gap-3"><span className="flex items-start gap-2"><input type="radio" name="payout-method" checked={method.id === methodID} onChange={() => { setMethodID(method.id); setRecipient({}); setNotice(""); }} /><span><span className="font-medium">{t(method.name)}</span><span className="mt-1 block text-xs text-ink-mute">{t(method.kind_label)}</span></span></span>
            {quote?.ok ? <span className="text-right text-sm"><span className="block text-ink-mute">{t("手续费")} <Money value={quote.fee} unit="MONEY" /></span><span className="mt-1 block font-medium">{t(receivedLabel(walletName))} {method.payout_currency ? <span>{previewPayout(quote.net,method.payout_rate || "1",method.payout_digits ?? digits) || "—"} {method.payout_currency}</span> : <Money value={quote.net} unit="MONEY" />}</span></span> : quote ? <span className="text-xs text-danger">{t(payoutReason(quote.reason, walletName))}</span> : <span className="text-xs text-ink-mute">{method.fee_rate !== "0" ? t("按比例收取手续费") : t("按固定金额收取手续费")}</span>}
          </span>
        </label>;
      })}</fieldset>
      {selected && <div className="space-y-4"><div><h3 className="text-sm font-medium">{t("收款资料")}</h3><p className="mt-2 text-sm leading-6 text-ink-secondary">{t(selected.summary)}</p>{selected.payout_note && <p className="mt-2 text-sm whitespace-pre-wrap">{selected.payout_note}</p>}{selected.payout_currency && <p className="mt-2 text-xs">{t("提现兑换比例")} · 1 {monetary.currency} = {selected.payout_rate} {selected.payout_currency}</p>}</div>{selected.fields.map(field => <div key={field.name} className="text-sm"><label className="block">{t(field.label)}{field.optional && ` (${t("可选")})`}{field.kind === "select" ? <select className="mt-2 h-11 w-full rounded-control border border-hairline bg-canvas-raised px-3" value={recipient[field.name] || ""} onChange={event => setRecipient(current => ({ ...current, [field.name]: event.target.value }))} required={!field.optional}><option value="">{t("请选择")}</option>{field.options?.map(option => <option key={option.value} value={option.value}>{t(option.label)}</option>)}</select> : <Input className="mt-2" value={recipient[field.name] || ""} onChange={event => setRecipient(current => ({ ...current, [field.name]: event.target.value }))} required={!field.optional} />}</label>{field.help && <p className="mt-2 text-xs leading-6 text-ink-mute">{t(field.help)}</p>}</div>)}
        {preview?.ok && <p className="text-sm leading-7">{t("本次申请总额")} <Money value={preview.amount} unit="MONEY" /> · {t("手续费")} <Money value={preview.fee} unit="MONEY" /> · {t(receivedLabel(walletName))} {selected.payout_currency ? <span>{payoutAmount || "—"} {selected.payout_currency}</span> : <Money value={preview.net} unit="MONEY" />}</p>}
        {preview && !preview.ok && <p role="alert" className="text-sm text-danger">{t(payoutReason(preview.reason, walletName))}</p>}
      </div>}
      </fieldset>
      {!!command.error && <ErrorPanel error={command.error} />}
      {(command.unresolved || blocked) && <Notice>{t("请保持此办理面板，查询业务记录或重试本次操作。重试会复用原提交内容与同一操作编号，避免重复办理。")}</Notice>}
      {notice && <Notice>{t(notice)}</Notice>}
      <Button type="button" disabled={(!ready && !command.unresolved) || command.busy} onClick={() => void submit()}>{command.busy ? t("正在提交…") : command.unresolved ? t("重试本次操作") : t("提交提现申请")}</Button>
    </div>}
  </Card>;
}

export function CustomerWithdrawalApplications({ rows, cancel, onCancel, walletName = "佣金钱包" }: { rows: Entity[]; cancel?: Operation; onCancel: (operation: Operation, row: Entity) => void; walletName?: string }) {
  const { t } = useI18n();
  return <div className="mt-8 space-y-3"><h2 className="text-lg font-semibold">{t("提现申请")}</h2><p className="text-sm leading-6 text-ink-secondary">{t(walletName === "支付余额" ? "尚未审核的申请可以取消。取消后释放这笔支付余额占用。" : "尚未审核的申请可以取消。取消后释放这笔佣金占用。")}</p>{!rows.length ? <Notice>{t("还没有提现申请。")}</Notice> : rows.map(row => <Card key={row.id}><div className="flex flex-wrap items-start justify-between gap-3"><div><p className="font-medium">{t(String(row.values.method_name || "提现申请"))}</p><p className="mt-2 whitespace-pre-wrap text-sm leading-6 text-ink-secondary">{payoutRecipientText(String(row.values.recipient_reference || ""), t)}</p></div>{row.status && <Status value={row.status} labels={withdrawalStatusLabels} />}</div><div className="mt-4 grid gap-3 text-sm sm:grid-cols-3"><div><p className="text-xs text-ink-mute">{t("总额")}</p><Money value={String(row.values.amount_money || "0")} unit="MONEY" /></div><div><p className="text-xs text-ink-mute">{t("手续费")}</p><Money value={String(row.values.fee_money || "0")} unit="MONEY" /></div><div><p className="text-xs text-ink-mute">{t(receivedLabel(walletName))}</p>{row.values.payout_currency ? <span>{String(row.values.payout_amount)} {String(row.values.payout_currency)}</span> : <Money value={String(row.values.net_money || "0")} unit="MONEY" />}</div></div>{cancel && row.allowed_actions?.includes("cancel") && <Button type="button" variant="outline" className="mt-4" onClick={() => onCancel(cancel, row)}>{t(cancel.label)}</Button>}</Card>)}
  </div>;
}
