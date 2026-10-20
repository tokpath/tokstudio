"use client";
import { useState } from "react";
import { ArrowRight } from "lucide-react";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import { OrderSchema, type Order } from "@/lib/contracts";
import { Money, Notice, useCommand } from "./shared";
import { useProduct } from "./context";

const paymentWindowName = "tokpath-gkash";

// The current page stays here so the payer can read the amount and pick another
// method. The Gkash form itself is posted into a separate window.
export function GkashPay({ action, order, onChangeMethod }: { action: NonNullable<Order["payment_action"]>; order: Order; onChangeMethod?: () => void }) {
  const { t } = useI18n();
  const { context, session } = useProduct();
  const [nextMethod, setNextMethod] = useState("");
  const [choosing, setChoosing] = useState(false);
  const replace = useCommand(`/api/v1/me/orders/${encodeURIComponent(order.id)}/payment-method`, OrderSchema);
  const href = action.url || "";
  const fields = action.fields || {};
  const methods = (context?.payment_methods || []).filter(item => item.status === "ready" && item.id !== "");
  const others = methods.filter(item => item.name !== "kvoucher");
  const customerCanSwitch = session?.kind === "customer" && others.length > 0;
  return <div className="space-y-4">
    <DetailAmount action={action} />
    <Notice>{t("Gkash 付款页在新窗口打开，本页会留下来。请先看清付款页金额；不合适就改用其它付款方式。如果已经在 Gkash 付过，请先核实原通道，不要更换。")}</Notice>
    <form method="POST" action={href} target={paymentWindowName} className="flex flex-wrap gap-3">
      {Object.entries(fields).map(([name, value]) => <input key={name} type="hidden" name={name} value={value} />)}
      <Button type="submit">{t("重新打开 Gkash 付款")}<ArrowRight /></Button>
    </form>
    {(customerCanSwitch || onChangeMethod) && <div className="space-y-3 border-t border-hairline pt-4">
      {!choosing && <Button type="button" variant="outline" onClick={() => onChangeMethod ? onChangeMethod() : setChoosing(true)}>{t("改用其它付款方式")}</Button>}
      {choosing && customerCanSwitch && <form className="space-y-3" onSubmit={async event => { event.preventDefault(); if (!nextMethod || replace.busy) return; await replace.run({ external_method_id: nextMethod }); setChoosing(false); }}>
        <label className="block text-sm">{t("选择其它付款方式")}<select required value={nextMethod} onChange={event => setNextMethod(event.target.value)} className="mt-2 h-11 w-full rounded-control border border-hairline bg-canvas-raised px-3"><option value="">{t("选择已就绪的支付方式")}</option>{others.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
        <div className="flex flex-wrap gap-3"><Button type="submit" disabled={replace.busy || !nextMethod}>{replace.busy ? t("正在更换付款方式…") : t("确认更换付款方式")}</Button><Button type="button" variant="ghost" onClick={() => setChoosing(false)}>{t("留下当前付款方式")}</Button></div>
      </form>}
    </div>}
  </div>;
}

function DetailAmount({ action }: { action: NonNullable<Order["payment_action"]> }) {
  const { t } = useI18n();
  if (!action.amount) return null;
  return <p className="text-sm leading-7">{t("付款页金额")}<span className="ml-2 font-medium"><Money value={action.amount} unit={action.currency || ""} /></span><span className="mt-1 block text-xs text-ink-mute">{t("这个金额含通道手续费。本订单仍按上方应付美元入账。")}</span></p>;
}
