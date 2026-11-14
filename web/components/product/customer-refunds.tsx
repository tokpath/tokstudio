"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { request } from "@/lib/api-client";
import { collection, CommandResultSchema, RefundDestinationSchema, type Order } from "@/lib/contracts";
import type { z } from "zod";
import { ErrorPanel, Money, Notice, Status, When, useCommand } from "./shared";

const refundStates: Record<string, string> = { approved: "已批准待执行", paying: "处理中", completed: "已核实退款" };

export function CustomerOrderRefunds({ refunds, currency }: { refunds: NonNullable<Order["refunds"]>; currency: string }) {
  const { t } = useI18n();
  return <section className="space-y-4 rounded-card border border-hairline p-5">
    <h4 className="font-semibold">{t("退款进度")}</h4>
    {refunds.map(refund => <div key={refund.id} className="space-y-3 border-t border-hairline pt-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Money value={refund.amount_money} unit={currency} />
        <Status value={refund.status} labels={refundStates} />
      </div>
      <p className="text-xs text-ink-secondary"><When value={refund.paid_at || refund.created_at} /></p>
      {refund.status === "completed" && refund.paid_at && <a href={`/api/v1/me/refunds/${encodeURIComponent(refund.id)}/voucher`} className="text-sm text-brand-emphasis underline" download>{t("下载退款凭证")}</a>}
      {["pending_review", "approved", "paying"].includes(refund.status) && <RefundDestinations caseID={refund.id} />}
    </div>)}
  </section>;
}

function RefundDestinations({ caseID }: { caseID: string }) {
  const { t } = useI18n();
  const path = `/api/v1/me/refunds/${encodeURIComponent(caseID)}/destinations`;
  const query = useQuery({ queryKey: ["refund-destinations", caseID], queryFn: ({ signal }) => request(path, collection(RefundDestinationSchema), { signal }) });
  if (query.isError) return <ErrorPanel error={query.error} retry={() => void query.refetch()} compact />;
  return query.data?.items.filter(leg => leg.destination_required).map(leg => {
    if (leg.authorization_id) return <Notice key={leg.leg_id}>{t("退款收款信息已核实。")}</Notice>;
    if (leg.destination_mutable) return <RefundDestinationForm key={leg.leg_id} leg={leg} refresh={() => void query.refetch()} />;
    return null;
  });
}

function RefundDestinationForm({ leg, refresh }: { leg: z.infer<typeof RefundDestinationSchema>; refresh: () => void }) {
  const { t } = useI18n();
  const asset = leg.currency || leg.proposal?.asset || "";
  const [address, setAddress] = useState(leg.proposal?.address || "");
  const account = address.replace(/\s/g, "");
  const [agreed, setAgreed] = useState(false);
  const command = useCommand(`/api/v1/me/refunds/legs/${encodeURIComponent(leg.leg_id)}/responsible-destination`, CommandResultSchema);
  const confirmation = "我确认所填账户已获原付款人授权接收本笔退款。";
  return <form className="space-y-3 rounded-control border border-hairline p-4" onSubmit={async event => {
    event.preventDefault();
    if (!command.unresolved && (!agreed || !asset || !account)) return;
    const data = { destination: { network: "bank", asset, address: account }, authorization_reference: confirmation };
    const result = await command.run(command.unresolved ? undefined : data);
    if (result) {
      setAgreed(false);
      refresh();
    }
  }}>
    <fieldset className="space-y-3" disabled={command.busy || command.unresolved}>
      <legend className="font-medium">{t("确认退款收款信息")}</legend>
      {(leg.proposal || command.result) && <Notice>{t("收款信息已提交，等待经营方核实。")}</Notice>}
      <p className="text-sm">{t("收款币种")} · {asset}</p>
      <label className="block text-sm">{t("收款账号")}<Input className="mt-2" maxLength={256} value={address} onChange={event => setAddress(event.target.value)} required /></label>
      <label className="flex items-start gap-2 text-sm"><input type="checkbox" checked={agreed} onChange={event => setAgreed(event.target.checked)} className="mt-1" />{t(confirmation)}</label>
    </fieldset>
    {!!command.error && <ErrorPanel error={command.error} compact />}
    <Button type="submit" disabled={command.busy || (!command.unresolved && (!agreed || !asset || !account))}>{t(command.unresolved ? "重试本次操作" : "确认退款收款信息")}</Button>
  </form>;
}
