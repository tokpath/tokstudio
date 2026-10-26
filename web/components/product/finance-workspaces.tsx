"use client";

import { useEffect, useState } from "react";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import { Heading } from "./shared";
import { ResourceScreen } from "./operator-resources";

export function FinanceReceiptsScreen({ operatorID }: { operatorID: string }) {
  const { t } = useI18n();
  const [pending, setPending] = useState(false);
  useEffect(() => {
    const read = () => setPending(new URLSearchParams(window.location.search).get("section") === "pending");
    read(); window.addEventListener("popstate", read); return () => window.removeEventListener("popstate", read);
  }, []);
  function select(value: boolean) {
    setPending(value);
    const url = new URL(window.location.href); url.search = "";
    if (value) url.searchParams.set("section", "pending");
    window.history.pushState(window.history.state, "", url);
  }
  return <>
    <Heading title={t("收款办理")} description={t("核实到账、关联订单并跟进交付。原币尚未核实的款项在同一处继续办理。")} />
    <div className="mb-6 flex gap-2" aria-label={t("收款视图")}>
      <Button variant={pending ? "outline" : "default"} onClick={() => select(false)}>{t("已入账收款")}</Button>
      <Button variant={pending ? "default" : "outline"} onClick={() => select(true)}>{t("待核实收款")}</Button>
    </div>
    <ResourceScreen key={pending ? "pending" : "receipts"} resource={pending ? "receipt-valuations" : "payments"} operatorID={operatorID} embedded inspectionLinks />
  </>;
}

export function PaymentMethodsScreen({ operatorID }: { operatorID: string }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  return <>
    <Heading title={t("收款方式")} description={t("配置实际收款通道。外币付款金额需要报价依据时，再展开下方设置。")} />
    <ResourceScreen operatorID={operatorID} resource="payment-methods" embedded layout="cards" inspectionLinks />
    <details className="mt-8 rounded-card border border-hairline bg-canvas-raised p-5" onToggle={event => setOpen(event.currentTarget.open)}>
      <summary className="cursor-pointer text-sm font-medium">{t("外币收款报价依据")}</summary>
      {open && <div className="mt-5"><p className="mb-4 text-sm leading-7 text-ink-secondary">{t("用于向付款人展示外币应付金额。实际收款仍按真实付款时点核实入账，不锁汇。USD 收款不需要此配置。")}</p><ResourceScreen resource="valuations" operatorID={operatorID} embedded inspectionLinks /></div>}
    </details>
  </>;
}
