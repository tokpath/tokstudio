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
    <Heading title={t("支付方式与手续费")} description={t("每种支付方式只设置一套手续费，统一用于它支持的收款、余额提现和佣金提现。")} />
    <div className="mb-6 grid gap-4 md:grid-cols-3">{[
      ["收款：本金之外另加", "充值、直接购买套餐和下级采购，共用所选方式的规则。支付 100 本金、手续费 2 时，合计付款 102，权益按 100 交付。"],
      ["提现：申请总额内扣", "余额和佣金提现共用同一规则。申请 100、手续费 2 时，账户扣除 100，实际付款 98。"],
      ["保存后影响哪些交易", "只影响新确认的付款和提现。已确认的金额保持不变；内部余额和佣金抵扣不收费。异常退款会退回本金及已收支付手续费。"]
    ].map(([title,detail])=><div key={title} className="rounded-control border border-hairline p-4"><h3 className="font-medium">{t(title)}</h3><p className="mt-2 text-sm leading-7 text-ink-secondary">{t(detail)}</p></div>)}</div>
    <ResourceScreen operatorID={operatorID} resource="payment-methods" embedded layout="cards" inspectionLinks />
  </>;
}
