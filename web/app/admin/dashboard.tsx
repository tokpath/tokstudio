"use client";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useMemo,useState } from "react";
import { useTheme } from "next-themes";
import { useTranslations } from "next-intl";
import { readUsage } from "@/components/usage-report";
import { useViewer } from "@/components/rbac/viewer-context";
import { Button } from "@/components/ui/button";
import { MetricCard } from "@/components/feature-card";
import { EChart } from "@/components/echart";
import { chartPalette,dailyChartOption,requestChartOption } from "@/lib/charts";
import { dashboardHero } from "@/lib/dashboard";
import { DASHBOARD_HERO_ICONS } from "@/lib/page-icons";

type Body={dashboard:{totals?:Record<string,number>;generated_at?:string;module_errors?:Record<string,string>;alerts?:{kind?:string}[];provider_health?:{state?:string};dimensions?:Record<string,{key:string;requests?:number;revenue_minor?:number;success_rate?:number}[]>}};
type Series={items:{day:string;requests?:number;revenue_minor?:number;success_rate?:number}[]};
export default function AdminDashboard(){
 const t=useTranslations("admin"),td=useTranslations("dashboard"),tw=useTranslations("workbench"),tc=useTranslations("charts"),viewer=useViewer();
 const {resolvedTheme}=useTheme();const[dimension,setDimension]=useState("model");
 const query=useQuery({queryKey:[viewer.userId,"dashboard"],queryFn:()=>readUsage<Body>("/admin/ops/dashboard"),enabled:viewer.signedIn,retry:false});
 const series=useQuery({queryKey:[viewer.userId,"metrics-series"],queryFn:()=>readUsage<Series>("/admin/metrics/series?days=7"),enabled:viewer.signedIn,retry:false});
 const dash=query.isError?undefined:query.data?.dashboard,errors=dash?.module_errors||{};
 const palette=useMemo(()=>chartPalette(resolvedTheme==="dark"),[resolvedTheme]),labels=useMemo(()=>({requests:tc("requests"),revenue:tc("spend")}),[tc]);
 const dims=dash?.dimensions?.[dimension];
 const dimOption=useMemo(()=>dims?.length?requestChartOption(dims,labels,palette):null,[dims,labels,palette]);
 const dayOption=useMemo(()=>series.data?.items.length?dailyChartOption(series.data.items,labels,palette):null,[series.data,labels,palette]);
 const cards=dashboardHero(dash||{});const dimFailed=!!errors[`dimensions.${dimension}`];
 const retry=()=>{void query.refetch();void series.refetch()};
 return <div className="flex flex-col gap-5">
  {query.isError?<section role="alert" className="rounded-card border border-hairline p-4"><p>{tw("readFailed")}</p><Button variant="outline" onClick={retry}>{tw("retry")}</Button></section>:null}
  <section className="grid gap-4 md:grid-cols-2 lg:grid-cols-4" aria-label={tw("tasks")}>{cards.map(card=><MetricCard key={card.key} icon={DASHBOARD_HERO_ICONS[card.key]??DASHBOARD_HERO_ICONS.heroPending} label={t(card.key)} value={card.key==="heroHealth"?tw(errors.provider_health?"readFailed":card.v):card.v} hint={t(card.hintKey)} href={card.href}/>)}</section>
  <section className="rounded-card border border-hairline bg-canvas-raised p-5"><div className="flex flex-wrap items-center justify-between gap-3"><h2 className="font-semibold">{tw("tasks")}</h2><Button variant="outline" onClick={retry}>{td("refreshMetrics")}</Button></div><div className="mt-4 flex flex-wrap gap-4"><Link className="text-brand" href="/admin/models">{tw("models")}</Link><Link className="text-brand" href="/admin/channels">{tw("delivery")}</Link><Link className="text-brand" href="/admin/payments">{tw("payments")}</Link><Link className="text-brand" href="/admin/usage?tab=requests">{tw("requests")}</Link></div>{Object.keys(errors).length?<p role="status" className="mt-4 text-sm text-hold">{tw("partialFailure")}</p>:null}{dash?.generated_at?<p className="mt-3 text-xs text-ink-mute">{tw("updated")} {new Date(dash.generated_at).toLocaleString(undefined,{timeZone:"Asia/Shanghai"})} · Asia/Shanghai · {tw("cumulative")}</p>:null}</section>
  <section className="rounded-card border border-hairline bg-canvas-raised p-5"><label className="flex items-center gap-2 text-sm">{td("dimLabel")}<select className="h-9 rounded-control border border-hairline bg-canvas px-2" value={dimension} onChange={e=>setDimension(e.target.value)}>{["model","api_key","channel","user","provider"].map(v=><option key={v} value={v}>{td({model:"dimModel",api_key:"dimApiKey",channel:"dimChannel",user:"dimUser",provider:"dimProvider"}[v]!)}</option>)}</select></label>
   <div className="mt-4 grid gap-4 lg:grid-cols-2"><section><p className="mb-2 text-sm">{tw("cumulative")}</p>{query.isPending?<p role="status">{tw("loading")}</p>:query.isError||dimFailed?<p role="alert">{tw("readFailed")}</p>:<EChart option={dimOption} emptyTitle={tc("empty")} emptyDetail={tc("emptyDetail")} testId="ops-echarts"/>}</section><section><p className="mb-2 text-sm">{td("last7d")} · UTC</p>{series.isPending?<p role="status">{tw("loading")}</p>:series.isError?<div role="alert"><p>{tw("readFailed")}</p><Button variant="outline" onClick={()=>void series.refetch()}>{tw("retry")}</Button></div>:<EChart option={dayOption} emptyTitle={tc("empty")} emptyDetail={tc("emptyDetail")} testId="ops-daily-chart"/>}</section></div>
  </section>
 </div>
}
