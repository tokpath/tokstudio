"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { useTheme } from "next-themes";
import { useViewer } from "@/components/rbac/viewer-context";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { EChart } from "@/components/echart";
import { AdminListPanel } from "@/app/admin/list-panel";
import { apiBase } from "@/lib/api";
import { formatUsdMinor } from "@/lib/money";
import { chartPalette, dailyChartOption, requestChartOption } from "@/lib/charts";
import { safeReturnHref } from "@/lib/return-context";

export type UsageSurface = "user" | "admin" | "channel";
export type UsageTotals = { requests: number; confirmed: number; pending: number; voided: number; amount_minor: number; prompt_tokens: number; completion_tokens: number; reasoning_tokens: number };
type UsageGroup = UsageTotals & { key: string };
type Summary = { totals: UsageTotals; daily: UsageGroup[]; keys: UsageGroup[]; models: UsageGroup[]; time_zone: string; generated_at: string; key_labels?:Record<string,string>; facets?:{keys:string[];models:string[]}; customer?:{id:string;email:string;display_name:string} };
type Filters = { from: string; to: string; api_key_id: string; public_model_id: string; user_id: string; state: string; tab: string; request_id: string };
export type RequestReceipt = Record<string, unknown> & { request_id: string; public_model_id: string; api_key_id?: string; result: string; billing_state?: string; customer_amount_minor: number; error_code?: string; started_at: string };

export function usageEndpoint(surface: UsageSurface) { return surface === "user" ? "/v1/me" : `/${surface}`; }
export function usageConsole(surface: UsageSurface) { return surface === "user" ? "/app" : `/${surface}`; }
export async function readUsage<T>(path: string): Promise<T> {
 const response = await fetch(`${apiBase}${path}`, { credentials: "include" });
 const body = await response.json();
 if (!response.ok) throw new Error(body.error?.message || "read_error");
 return body as T;
}
function today() { return new Intl.DateTimeFormat("sv-SE", {timeZone:"Asia/Shanghai"}).format(new Date()); }
function initial(): Filters {
 const to = today(); const date = new Date(`${to}T00:00:00+08:00`); date.setUTCDate(date.getUTCDate()-6);
 return {from:new Intl.DateTimeFormat("sv-SE",{timeZone:"Asia/Shanghai"}).format(date),to,api_key_id:"",public_model_id:"",user_id:"",state:"",tab:"summary",request_id:""};
}
function label(t: ReturnType<typeof useTranslations>, status?: string) { const known=["confirmed","pending_reconciliation","voided","success","succeeded","failed","running","cancelled"];return status ? known.includes(status) ? t(status) : status : "—"; }

export function UsageReport({surface,channelID}: {surface:UsageSurface;channelID?:string}) {
 const t=useTranslations("usageWorkflow"), viewer=useViewer();
 const {resolvedTheme}=useTheme();
 const [filters,setFilters]=useState<Filters>(initial),[ready,setReady]=useState(false),[returnTo,setReturnTo]=useState<string|null>(null);
 useEffect(()=>{
  const restore=()=>{const p=new URLSearchParams(window.location.search);const defaults=initial();setFilters({...defaults,...Object.fromEntries(Object.keys(defaults).map(key=>[key,p.get(key) ?? (key==="api_key_id"?p.get("key"):key==="public_model_id"?p.get("model"):null) ?? defaults[key as keyof Filters]]))});setReturnTo(p.get("return_to") ? safeReturnHref(p.get("return_to"),`${usageConsole(surface)}/usage`) : null);setReady(true)};
  restore();window.addEventListener("popstate",restore);return()=>window.removeEventListener("popstate",restore);
 },[]);
 const params=useMemo(()=>{const p=new URLSearchParams();for(const [k,v] of Object.entries(filters)){if(v && k!=="tab")p.set(k,v)};p.set("time_zone","Asia/Shanghai");if(channelID)p.set("channel_id",channelID);return p;},[filters,channelID]);
 const summaryParams=new URLSearchParams(params);summaryParams.delete("request_id");
 const endpoint=usageEndpoint(surface), consolePath=usageConsole(surface);
 const summary=useQuery({queryKey:[viewer.userId,endpoint,channelID,"usage-summary",summaryParams.toString()],queryFn:()=>readUsage<Summary>(`${endpoint}/usage/summary?${summaryParams}`),enabled:ready,retry:false});
 const report=summary.data;
 function update(key:keyof Filters,value:string){const next={...filters,[key]:value};setFilters(next);const u=new URL(window.location.href);for(const[k,v]of Object.entries(next)){if(v)u.searchParams.set(k,v);else u.searchParams.delete(k)};for(const k of [...u.searchParams.keys()])if(k.endsWith("_cursor"))u.searchParams.delete(k);window.history.replaceState(null,"",u.toString());}
 const palette=useMemo(()=>chartPalette(resolvedTheme==="dark"),[resolvedTheme]);
 const chartLabels=useMemo(()=>({requests:t("requests"),revenue:t("amount")}),[t]);
 const trend=useMemo(()=>report?.daily.length?dailyChartOption(report.daily.map(v=>({day:v.key,requests:v.requests,revenue_minor:v.amount_minor})),chartLabels,palette):null,[report,chartLabels,palette]);
 const breakdown=useMemo(()=>report?.models.length?requestChartOption(report.models.map(v=>({key:v.key,requests:v.requests,revenue_minor:v.amount_minor})),chartLabels,palette):null,[report,chartLabels,palette]);
 const requestHref=(row:RequestReceipt)=>`${consolePath}/usage/requests/${encodeURIComponent(row.request_id)}${channelID?`?channel_id=${encodeURIComponent(channelID)}`:""}`;
 return <div className="flex flex-col gap-5">
  {returnTo?<Link href={returnTo} className="text-sm text-brand">← {t("back")}</Link>:null}
  {filters.user_id&&surface!=="user"?<p className="text-sm">{t("customerScope")} <span>{report?.customer?.display_name||report?.customer?.email||filters.user_id}</span></p>:null}
  <section className="rounded-card border border-hairline bg-canvas-raised p-5">
   <div className="flex flex-wrap items-end gap-3">
    {(["from","to"]as const).map(key=><label key={key} className="grid gap-1 text-sm"><span>{t(key)}</span><Input type="date" aria-label={t(key)} value={filters[key]} onChange={e=>update(key,e.target.value)}/></label>)}
    <label className="grid gap-1 text-sm"><span>{t("key")}</span><select aria-label={t("key")} value={filters.api_key_id} onChange={e=>update("api_key_id",e.target.value)} className="h-10 rounded-control border border-hairline bg-canvas px-2"><option value="">{t("all")}</option>{[...new Set([filters.api_key_id,...report?.facets?.keys||report?.keys.map(v=>v.key)||[]])].filter(Boolean).map(key=><option key={key} value={key}>{report?.key_labels?.[key]||key}</option>)}</select></label>
    <label className="grid gap-1 text-sm"><span>{t("model")}</span><select aria-label={t("model")} value={filters.public_model_id} onChange={e=>update("public_model_id",e.target.value)} className="h-10 rounded-control border border-hairline bg-canvas px-2"><option value="">{t("all")}</option>{[...new Set([filters.public_model_id,...report?.facets?.models||report?.models.map(v=>v.key)||[]])].filter(Boolean).map(key=><option key={key} value={key}>{key}</option>)}</select></label>
    <label className="grid gap-1 text-sm"><span>{t("billing")}</span><select aria-label={t("billing")} value={filters.state} onChange={e=>update("state",e.target.value)} className="h-10 rounded-control border border-hairline bg-canvas px-2"><option value="">{t("all")}</option>{["confirmed","pending_reconciliation","voided"].map(v=><option key={v} value={v}>{t(v)}</option>)}</select></label>
    <Button variant="outline" onClick={()=>void summary.refetch()}>{t("refresh")}</Button>
   </div><p className="mt-3 text-xs text-ink-mute">{t("scope",{from:filters.from||t("all"),to:filters.to||t("all"),zone:"Asia/Shanghai"})}</p>
  </section>
  <nav aria-label={t("tabs")} className="flex gap-2">{["summary","requests"].map(tab=><Button key={tab} variant={filters.tab===tab?"default":"outline"} onClick={()=>update("tab",tab)}>{t(tab)}</Button>)}</nav>
  {filters.tab==="requests"?<AdminListPanel<RequestReceipt> key={`${endpoint}:${params}`} path={`${endpoint}/requests?${params}`} title={t("requests")} rowHref={requestHref} emptyTitle={t("emptyRequests")} emptyDetail={t("adjust")}
   columns={[{accessorKey:"started_at",header:t("time"),cell:({row})=>new Date(row.original.started_at).toLocaleString(undefined,{timeZone:"Asia/Shanghai"})},{accessorKey:"public_model_id",header:t("model")},{accessorKey:"result",header:t("result"),cell:({row})=>label(t,row.original.result)},{accessorKey:"billing_state",header:t("billing"),cell:({row})=>label(t,row.original.billing_state)},{accessorKey:"customer_amount_minor",header:t("amount"),cell:({row})=>formatUsdMinor(row.original.customer_amount_minor)},{accessorKey:"request_id",header:t("request")}]} />:<>
   <section className="rounded-card border border-hairline bg-canvas-raised p-5" data-testid="list-resource-usage" data-list-phase={summary.isPending?"loading":summary.isError?"error":report?.totals.requests?"ready":"empty"}>
    {summary.isPending?<p role="status">{t("loading")}</p>:summary.isError?<div role="alert"><p>{t("readFailed")}</p><Button variant="outline" onClick={()=>void summary.refetch()}>{t("retry")}</Button></div>:null}
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">{[{key:"requests",value:report?.totals.requests,id:"usage-stat-requests"},{key:"amount",value:report?formatUsdMinor(report.totals.amount_minor):undefined,id:"usage-stat-spend"},{key:"prompt",value:report?.totals.prompt_tokens},{key:"completion",value:report?.totals.completion_tokens}].map(v=><div key={v.key}><p className="text-sm text-ink-mute">{t(v.key)}</p><p className="mt-2 font-mono text-2xl" data-testid={v.id}>{summary.isError||summary.isPending?"—":v.value??"—"}</p></div>)}</div>
    {report&&!summary.isError?<p className="mt-4 text-sm">{t("states",{confirmed:report.totals.confirmed,pending:report.totals.pending,voided:report.totals.voided})}</p>:null}
   </section>
   {report&&!summary.isError?<><div className="grid gap-4 lg:grid-cols-2"><section className="rounded-card border border-hairline bg-canvas-raised p-4"><h2>{t("trend")}</h2><EChart option={trend} emptyTitle={t("empty")} emptyDetail={t("adjust")} testId="usage-trend-chart"/></section><section className="rounded-card border border-hairline bg-canvas-raised p-4"><h2>{t("byModel")}</h2><EChart option={breakdown} emptyTitle={t("empty")} emptyDetail={t("adjust")} testId="usage-breakdown-chart"/></section></div>
    <section className="overflow-x-auto rounded-card border border-hairline bg-canvas-raised p-5"><h2>{t("byKey")}</h2><table className="mt-3 w-full text-left text-sm"><thead><tr>{["key","requests","amount"].map(v=><th className="p-2" key={v}>{t(v)}</th>)}</tr></thead><tbody>{report.keys.map(v=><tr key={v.key}><td className="p-2"><span>{report.key_labels?.[v.key]||v.key||"—"}</span>{report.key_labels?.[v.key]?<details className="text-xs text-ink-mute"><summary>{t("identifier")}</summary><button type="button" onClick={()=>void navigator.clipboard.writeText(v.key)} className="break-all font-mono">{v.key}</button></details>:null}</td><td className="p-2">{v.requests}</td><td className="p-2" data-testid={`usage-key-amount-${v.key||"none"}`}>{formatUsdMinor(v.amount_minor)}</td></tr>)}</tbody></table>{!report.totals.requests?<p className="mt-3">{t("empty")}</p>:null}</section></>:null}
  </>}
 </div>;
}
