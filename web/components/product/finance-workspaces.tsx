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
  return <>
    <Heading title={t("收款方式")} description={t("配置实际收款通道。原币收款与已核实的结算金额分别记录，不设置汇率。")} />
    <ResourceScreen operatorID={operatorID} resource="payment-methods" embedded layout="cards" inspectionLinks />
  </>;
}
