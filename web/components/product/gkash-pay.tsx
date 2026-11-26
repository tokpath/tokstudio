"use client";
import { useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ArrowRight } from "lucide-react";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import { OrderSchema, type Order } from "@/lib/contracts";
import { closePaymentWindow, customerGkashOpenPath, focusPaymentWindow, openGkashPayment, type PaymentWindow, type PaymentWindowState } from "@/lib/payment-window";
import { ErrorPanel, Notice, useCommand } from "./shared";
import { useProduct } from "./context";

export type GkashLaunchState = { busy: boolean; message?: string; error?: unknown };

export function useGkashLaunch() {
  const cache = useQueryClient();
  const running = useRef(false);
  const [state, setState] = useState<GkashLaunchState>();
  async function run(order: Order, target?: PaymentWindow | null, path = customerGkashOpenPath(order.id)) {
    if (running.current) {
      closePaymentWindow(target ?? null);
      return;
    }
    running.current = true;
    setState({ busy: true });
    try {
      const message = await openGkashPayment(order, path, target);
      setState({ busy: false, message });
    } catch (error) {
      setState({ busy: false, error });
    } finally {
      running.current = false;
      const queryKey = order.kind === "procurement" ? ["procurement-payment"] : ["order-detail", order.id];
      void cache.invalidateQueries({ queryKey });
    }
  }
  return { state, run };
}

export function GkashPay({ order, windowState, openPath, launch, busy, onChangeMethod }: {
  order: Order;
  windowState: PaymentWindowState;
  openPath?: string;
  launch: ReturnType<typeof useGkashLaunch>;
  busy: boolean;
  onChangeMethod?: () => void;
}) {
  const { t } = useI18n();
  const { context, session } = useProduct();
  const [nextMethod, setNextMethod] = useState("");
  const [choosing, setChoosing] = useState(false);
  const replace = useCommand(`/api/v1/me/orders/${encodeURIComponent(order.id)}/payment-method`, OrderSchema);
  const methods = (context?.payment_methods || []).filter(item => item.status === "ready" && item.settlement_supported !== false && item.id !== "");
  const others = methods.filter(item => item.name !== "kvoucher");
  const customerCanSwitch = order.method_change_allowed === true && session?.kind === "customer" && others.length > 0;
  const label = busy ? "正在打开 Gkash 付款页…" : windowState === "unopened" ? "前往 GKash 付款" : "重新打开 GKash 付款";
  function open() {
    if (busy || replace.busy || focusPaymentWindow(order.id) || !openPath) return;
    void launch.run(order, undefined, openPath);
  }
  return <div className="space-y-5">
    <Notice>{t("请在 Gkash 付款页确认最终金额。未付款可更换付款方式；已经付款请等待核实，勿重复付款。")}</Notice>
    {(windowState === "open" || openPath) && <Button type="button" onClick={open} disabled={busy || replace.busy}>{t(label)}<ArrowRight /></Button>}
    {(customerCanSwitch || order.method_change_allowed === true && onChangeMethod) && <div className="space-y-3 border-t border-hairline pt-4">
      {!choosing && <Button type="button" variant="outline" disabled={busy} onClick={() => onChangeMethod ? onChangeMethod() : setChoosing(true)}>{t("改用其它付款方式")}</Button>}
      {choosing && customerCanSwitch && <form className="space-y-3" onSubmit={async event => { event.preventDefault(); if (!nextMethod || replace.busy || busy) return; await replace.run({ external_method_id: nextMethod,fee_rule_id:others.find(item=>item.id===nextMethod)?.fee_rule_id }); setChoosing(false); }}>
        <label className="block text-sm">{t("选择其它付款方式")}<select required value={nextMethod} onChange={event => setNextMethod(event.target.value)} className="mt-2 h-11 w-full rounded-control border border-hairline bg-canvas-raised px-3"><option value="">{t("选择已就绪的支付方式")}</option>{others.map(item => <option key={item.id} value={item.id}>{item.display_name || item.name}</option>)}</select></label>
        <div className="flex flex-wrap gap-3"><Button type="submit" disabled={replace.busy || busy || !nextMethod}>{replace.busy ? t("正在更换付款方式…") : t("确认更换付款方式")}</Button><Button type="button" variant="ghost" onClick={() => setChoosing(false)}>{t("留下当前付款方式")}</Button></div>
      </form>}
    </div>}
    {replace.error ? <ErrorPanel error={replace.error} /> : null}
  </div>;
}
