"use client";

import { useEffect, useId, useState } from "react";
import { useI18n } from "@/components/i18n";
import { canReadResource } from "@/lib/navigation";
import { allowNavigation } from "@/lib/navigation-guard";
import { Button } from "@/components/ui/button";
import { useProduct } from "./context";
import { ResourceScreen } from "./operator-resources";
import { Empty, Heading } from "./shared";

const sections = [
  { id: "rules", label: "分佣规则", resource: "commissions", view: "", description: "设置注册赠送、KOL 资格门槛、两级分佣和最低提现金额。" },
  { id: "methods", label: "提现方式", resource: "withdrawals", view: "methods", description: "设置可用提现方式及固定手续费或手续费率。申请时固定本次费用和实付金额，线下按实付金额付款，无需逐笔登记实际手续费。" },
  { id: "requests", label: "提现申请", resource: "withdrawals", view: "requests", description: "审核佣金提现并线下付款，核实实际付款后完成扣账；结果未知时继续核实原付款。" },
] as const;

function location(record?: string) {
  const params = new URLSearchParams(typeof window === "undefined" ? "" : window.location.search);
  return { section: params.get("section") || "", record: params.get("record") || record };
}

export function OperatorCommissionsScreen({ operatorID, record }: { operatorID: string; record?: string }) {
  const { t } = useI18n();
  const { session } = useProduct();
  const workspace = session?.kind === "staff" ? session.workspaces.find(item => item.operator.id === operatorID) : undefined;
  const [current, setCurrent] = useState(() => location(record));
  const panel = useId();
  const available = sections.filter(item => canReadResource(workspace, item.resource));
  const selected = available.find(item => item.id === current.section) || available.find(item => item.id === "rules") || available.find(item => item.id === "requests");
  useEffect(() => {
    const restore = () => setCurrent(location());
    window.addEventListener("popstate", restore);
    return () => window.removeEventListener("popstate", restore);
  }, []);
  useEffect(() => { setCurrent(location(record)); }, [operatorID, record]);
  useEffect(() => {
    if (!selected) return;
    const url = new URL(window.location.href);
    url.searchParams.set("section", selected.id);
    window.history.replaceState(window.history.state, "", url);
  }, [selected?.id]);
  if (!selected) return <Empty title={t("没有查看权限")}>{t("此页面需要当前经营方明确授予相应权限。")}</Empty>;
  const select = (section: string) => {
    if (section === selected.id || !allowNavigation()) return;
    const url = new URL(window.location.href);
    for (const key of ["q", "status", "cursor", "previous", "view", "record", "returnTo"]) url.searchParams.delete(key);
    url.searchParams.set("section", section);
    window.history.pushState(window.history.state, "", url);
    setCurrent({ section, record: undefined });
  };
  return <>
    <Heading eyebrow={t("营销与推广")} title={t("邀请与分佣")} description={t("注册奖励、分佣规则和佣金提现在此统一管理。")}/>
    <div role="tablist" aria-label={t("邀请与分佣事项")} className="mb-5 flex flex-wrap gap-2">{available.map(item => <Button key={item.id} id={`${panel}-${item.id}`} role="tab" aria-selected={selected.id === item.id} aria-controls={panel} variant={selected.id === item.id ? "default" : "outline"} onClick={() => select(item.id)}>{t(item.label)}</Button>)}</div>
    <section role="tabpanel" id={panel} aria-labelledby={`${panel}-${selected.id}`}>
      <p className="mb-5 text-sm leading-7 text-ink-secondary">{t(selected.description)}</p>
      <ResourceScreen key={`${operatorID}:${selected.id}:${current.record || "all"}`} operatorID={operatorID} resource={selected.resource} controlledView={selected.view} embedded record={selected.id === "requests" ? current.record : undefined}/>
    </section>
  </>;
}
