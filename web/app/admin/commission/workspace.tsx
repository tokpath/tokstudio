"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { safeReturnHref } from "@/lib/return-context";
import { useTranslations } from "next-intl";
import { useViewer } from "@/components/rbac/viewer-context";
import { canChannelAction, canWrite } from "@/lib/rbac";
import { apiBase } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { SettlementPanel } from "./settlement-panel";
import { RecoveryPanel } from "./recovery-panel";
import { PolicyEditor } from "./policy-editor";
import { RecalcPanel } from "./recalc-panel";
import { OperationStatus, useCommissionOperation, type Context } from "./workflow-client";

export type WorkspaceProps = { scope: string; context: Context; prefix: "/admin" | "/channel"; params: URLSearchParams; update: (values: Record<string, string>) => void; writable: boolean; operation: ReturnType<typeof useCommissionOperation>; version: number };
const validTabs = ["pending", "payout", "recovery", "all", "rules", "verify"];
export function CommissionWorkspace({ oem = false }: { oem?: boolean }) {
  const t = useTranslations("commissionWorkflow");
  const viewer = useViewer();
  const prefix = oem ? "/channel" : "/admin";
  const [params, setParams] = useState(new URLSearchParams());
  const [context, setContext] = useState<Context | null>(null);
  const [failed, setFailed] = useState("");
  const [contextVersion, setContextVersion] = useState(0);
  const [version, setVersion] = useState(0);
  const [search, setSearch] = useState("");
  const writable = oem ? canChannelAction("finance", viewer) : canWrite("commission.write", viewer);
  const recoveryAllowed = oem ? viewer.roles.some(r => ["channel_admin", "oem_finance", "oem_audit", "platform_admin"].includes(r)) : canWrite("commission.recovery.read", viewer);
  const scope = context && viewer.userId ? `${viewer.userId}:${typeof window === "undefined" ? "" : window.location.host}:${context.owner_id}:${prefix}` : "";
  const operation = useCommissionOperation(scope, (payload, body) => { setVersion(v => v + 1); if(payload.kind==='settle'){const rows=body.items as {id:string}[];update({tab:'payout',q:'',status:'',cursor:'',settlement_id:rows?.length===1?rows[0].id:''});} });
  const tabValue = params.get("tab") || "pending";
  const tab = validTabs.includes(tabValue) ? tabValue : "pending";
  function update(values: Record<string, string>) {
    const next = new URLSearchParams(window.location.search);
    Object.entries(values).forEach(([key, value]) => value ? next.set(key, value) : next.delete(key));
    window.history.pushState(null, "", `${window.location.pathname}${next.size ? `?${next}` : ""}`); setParams(next);
  }
  useEffect(() => {
    const read = () => setParams(new URLSearchParams(window.location.search));
    read(); window.addEventListener("popstate", read); return () => window.removeEventListener("popstate", read);
  }, []);
  useEffect(() => setSearch(params.get("q") || ""), [params]);
  useEffect(() => {
    setContext(null); setFailed("");
    if (!viewer.userId || viewer.loading) return;
    let active = true;
    void fetch(`${apiBase}${prefix}/commission-context`, { credentials: "include" }).then(async response => {
      const body = await response.json(); if (!response.ok || !body.owner_id) throw new Error(body.error?.message || t("loadFailed"));
      if (active) setContext(body);
    }).catch(e => { if (active) setFailed(e instanceof Error ? e.message : t("loadFailed")); });
    return () => { active = false; };
  }, [viewer.userId, viewer.loading, prefix, contextVersion, t]);
  if (!context) return <section aria-busy={!failed} className="rounded-card border border-hairline p-6"><p role={failed ? "alert" : "status"}>{failed || t("loadingScope")}</p>{failed && <Button className="mt-3" variant="outline" onClick={() => setContextVersion(v => v + 1)}>{t("retry")}</Button>}</section>;
  const props: WorkspaceProps = { scope, context, prefix, params, update, writable, operation, version };
  const allowed = tab !== "recovery" || recoveryAllowed;
  return <div className="space-y-5">
    {params.has("return_to") && <Link className="text-sm text-brand-emphasis underline" href={safeReturnHref(params.get("return_to"), `${prefix}/commission`)}>{t("returnTask")}</Link>}
    <nav aria-label={t("workspace")} className="flex flex-wrap gap-2">{validTabs.filter(key => (key !== "recovery" || recoveryAllowed) && (key !== "verify" || writable)).map(key => <Button key={key} variant={tab === key ? "default" : "outline"} aria-pressed={tab === key} onClick={() => update({ tab: key, status: "", cursor: "" })}>{t(`tab.${key}`)}</Button>)}</nav>
    <div className="flex flex-wrap items-center gap-3 text-sm"><span>{t("brand")}: {context.owner_name || context.owner_code || context.owner_id}</span><label>{t("attribution")} <select aria-label={t("attribution")} className="rounded-control border border-hairline bg-canvas p-2" value={params.get("channel_id") || ""} onChange={e => update({ channel_id: e.target.value, cursor: "" })}><option value="">{t("allAttributions")}</option>{context.channel_ids.filter(Boolean).map(id => <option key={id} value={id}>{context.channel_codes[id] || id}</option>)}</select></label></div>
    <OperationStatus operation={operation} />
    {!allowed || (tab === "verify" && !writable) ? <p role="alert">{t("forbidden")}</p> : <>
      {!['rules', 'verify'].includes(tab) && <form className="flex flex-wrap gap-2" onSubmit={e => { e.preventDefault(); update({ q: search.trim(), cursor: "" }); }}><Input className="max-w-lg" aria-label={t("search")} placeholder={t("searchHint")} value={search} onChange={e => setSearch(e.target.value)} /><Button variant="outline" type="submit">{t("search")}</Button></form>}
      {tab === "recovery" ? <RecoveryPanel key={scope} {...props} /> : tab === "rules" ? <PolicyEditor key={scope} prefix={prefix} canEdit={writable} scope={scope} ownerLabel={context.owner_code || context.owner_id} /> : tab === "verify" ? <RecalcPanel key={scope} /> : <SettlementPanel key={scope} {...props} view={tab as "pending" | "payout" | "all"} />}
    </>}
  </div>;
}
