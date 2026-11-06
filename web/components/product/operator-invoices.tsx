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
 { id: "issued", label: "我方开票", resource: "invoices", description: "核对客户及下级经营方的申请，上传已开具的发票文件并登记结果。" },
 { id: "received", label: "我方收票", resource: "procurement-invoices", description: "按已完成采购订单的实付金额向实际供货方申请开票，查看办理进度并下载发票。" },
] as const;

function location(received = false) {
  const params = new URLSearchParams(typeof window === "undefined" ? "" : window.location.search);
  return { section: params.get("section") || (received ? "received" : "issued") };
}

export function OperatorInvoicesScreen({ operatorID, received = false }: { operatorID: string; received?: boolean }) {
  const { t } = useI18n();
  const { session } = useProduct();
  const workspace = session?.kind === "staff" ? session.workspaces.find(item => item.operator.id === operatorID) : undefined;
  const [current, setCurrent] = useState(() => location(received));
  const panel = useId();
  const available = sections.filter(item => canReadResource(workspace, item.resource));
  const selected = available.find(item => item.id === current.section) || available[0];
  useEffect(() => {
    const restore = () => setCurrent(location());
    window.addEventListener("popstate", restore);
    return () => window.removeEventListener("popstate", restore);
  }, []);
  useEffect(() => { setCurrent(location(received)); }, [operatorID, received]);
  useEffect(() => {
    if (!selected) return;
    const url = new URL(window.location.href);
    url.pathname = `/manage/${encodeURIComponent(operatorID)}/invoices`;
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
    setCurrent({ section });
  };
  return <>
    <Heading eyebrow={t("资金办理")} title={t("票据办理")} description={available.length > 1 ? t("我方开出的发票和向上级申请的发票在此办理。") : undefined}/>
    <div role="tablist" aria-label={t("票据办理事项")} className="mb-5 flex flex-wrap gap-2">{available.map(item => <Button key={item.id} id={`${panel}-${item.id}`} role="tab" aria-selected={selected.id === item.id} aria-controls={panel} variant={selected.id === item.id ? "default" : "outline"} onClick={() => select(item.id)}>{t(item.label)}</Button>)}</div>
    <section role="tabpanel" id={panel} aria-labelledby={`${panel}-${selected.id}`}>
      <p className="mb-5 text-sm leading-7 text-ink-secondary">{t(selected.id === "issued" && workspace?.operator.kind === "channel" ? "核对直属客户的申请，上传已开具的发票文件并登记结果。" : selected.description)}</p>
      <ResourceScreen key={`${operatorID}:${selected.id}`} operatorID={operatorID} resource={selected.resource} embedded/>
    </section>
  </>;
}
