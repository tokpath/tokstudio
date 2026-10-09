"use client";

import { useEffect,useState } from "react";
import { useSearchParams } from "next/navigation";
import { safeReturnHref } from "@/lib/return-context";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { useViewer } from "@/components/rbac/viewer-context";
import { canViewAdminHref,canViewChannelHref,canWrite } from "@/lib/rbac";
import { confirmHeaders } from "@/lib/confirm";
import { apiClient } from "@/lib/client";
import { Button } from "@/components/ui/button";
import { ConfirmButton } from "@/components/confirm-button";
import { ChannelModelsPanel } from "@/app/admin/channels/models-panel";
import { ChannelQuotaPanel } from "@/app/admin/channels/quota-panel";
import { ChannelAdminsPanel } from "@/app/admin/channels/admins-panel";

type Projection={delivery:{channel_org_id:string;phase:string;version:number;sales_mode:string;sell_plans:boolean;receiver_user_id?:string;handed_over_at?:string;handoff_evidence:Record<string,unknown>};channel:{id:string;code:string};brand:{id:string;name:string;primary_domain:string;api_domain:string;admin_domain:string};checks:{key:string;ready:boolean;reason:string;responsibility:string;href?:string}[];ready:boolean;checked_at:string;registration_url?:string;administrators:{user_id:string;email:string;login_at?:string}[];request_evidence?:{request_id:string;public_model_id:string;occurred_at:string}};
export function DeliveryPanel({channelID,own=false}:{channelID?:string;own?:boolean}){
 const returnTo=useSearchParams().get("return_to");
 const [busy,setBusy]=useState(false);
 const [phase,setPhase]=useState("configuring");
 const t=useTranslations("oemDelivery");const viewer=useViewer();const path=own?"/channel/delivery":`/admin/oem-deliveries/${encodeURIComponent(channelID||"")}`;
 const query=useQuery({queryKey:[viewer.userId,path],queryFn:async()=>{const data=await apiClient<{item?:Projection;error?:{message:string}}>("GET",path);if(data.error||!data.item)throw new Error(data.error?.message||t("loadFailed"));return data.item}});
 const [mode,setMode]=useState("offline");const [plans,setPlans]=useState(false);const [receiver,setReceiver]=useState("");const [message,setMessage]=useState("");
 const item=query.data;const editable=!own&&canWrite("channels.write",viewer);
 useEffect(()=>{if(item){setMode(item.delivery.sales_mode);setPhase(item.delivery.phase);setPlans(item.delivery.sell_plans);setReceiver(item.administrators.find(a=>a.login_at)?.user_id||"")}},[item]);
 async function mutate(action:string,payload:unknown,method:"POST"|"PATCH"="POST"){
  setBusy(true);
  try{const body=await apiClient<{error?:{message:string}}>(method,`${path}${action}`,{headers:confirmHeaders,body:JSON.stringify(payload)});if(body.error){setMessage(body.error.message);return false}const result=await query.refetch();setMessage(result.isError?t("savedReadFailed"):t(action==="/handoff"?"handedOver":"saved"));return true}catch{setMessage(t("resultUnknown"));return false}finally{setBusy(false)}
 }
 if(query.isPending)return <p role="status">{t("loading")}</p>;
 if(query.isError||!item)return <section><p role="alert">{t("loadFailed")}</p><Button variant="outline" onClick={()=>void query.refetch()}>{t("refresh")}</Button></section>;
 const historical=item.delivery.handoff_evidence as {checks?:Projection["checks"];administrators?:Projection["administrators"];request_id?:string};
 const historicalReceiver=historical.administrators?.find(admin=>admin.user_id===item.delivery.receiver_user_id);
 return <div className="grid gap-6">
 {!own?<Link className="text-brand-emphasis underline" href={safeReturnHref(returnTo,"/admin/channels")}>{t("back")}</Link>:null}
 {message?<p role="status" aria-live="polite" className="sticky top-16 z-10 rounded-control border border-hairline bg-canvas-raised p-3">{message}</p>:null}
 <header className="flex flex-wrap items-start justify-between gap-4"><div><h1 className="text-2xl font-semibold">{item.brand.name} · {t("title")}</h1><p className="mt-2 text-sm">{t(`phase.${item.delivery.phase}`)} · {item.ready?t("ready"):t("notReady")}</p><p className="mt-1 text-sm text-ink-secondary">{t("checkedAt",{time:new Date(item.checked_at).toLocaleString(undefined,{timeZone:"Asia/Shanghai"})})}</p></div><Button variant="outline" onClick={()=>void query.refetch()}>{t("refresh")}</Button></header>
 {item.delivery.handed_over_at?<section className="rounded-card border border-hairline bg-canvas-raised p-5"><p>{t("history",{time:new Date(item.delivery.handed_over_at).toLocaleString(undefined,{timeZone:"Asia/Shanghai"})})}</p><p className="mt-2 text-sm">{t("acceptedBy",{email:historicalReceiver?.email||item.delivery.receiver_user_id||"—"})}</p><ul className="mt-3 text-sm">{historical.checks?.map(check=><li key={check.key}>✓ {t(`check.${check.key}`)}</li>)}</ul>{historical.request_id?<p className="mt-3 break-all text-sm">{t("check.request")}: {historical.request_id}</p>:null}</section>:null}
 <section className="rounded-card border border-hairline bg-canvas-raised p-6"><h2 className="font-semibold">{t("checklist")}</h2><ul className="mt-3 divide-y divide-hairline">{item.checks.map(check=><li key={check.key} className="flex flex-wrap items-center justify-between gap-3 py-3"><div><p>{check.ready?"✓":"○"} {t(`check.${check.key}`)}</p><p className="mt-1 text-sm text-ink-secondary">{t(`owner.${check.responsibility}`)}{!check.ready?` · ${t(`reason.${check.reason}`)}`:""}</p></div>{check.href&&(own?canViewChannelHref(check.href,viewer):canViewAdminHref(check.href,viewer))?<Link className="text-sm text-brand-emphasis underline" href={check.href}>{t("continue")}</Link>:null}</li>)}</ul>
 {!own&&viewer.roles.some(role=>["platform_admin","tech_admin"].includes(role))?<Button disabled={busy} variant="outline" className="mt-4" onClick={()=>void mutate("/domains/check",{})}>{t("checkDomains")}</Button>:null}
 </section>
 <section className="rounded-card border border-hairline bg-canvas-raised p-6"><h2 className="font-semibold">{t("entrypoints")}</h2><dl className="my-3 grid gap-2 text-sm">{[[t("primary"),item.brand.primary_domain],[t("api"),item.brand.api_domain],[t("admin"),item.brand.admin_domain]].map(([label,domain])=><div key={label}><dt className="text-ink-secondary">{label}</dt><dd className="break-all">https://{domain}</dd></div>)}</dl>{item.registration_url?<div><p className="break-all text-sm">{item.registration_url}</p><Button className="mt-3" variant="outline" onClick={async()=>{try{await navigator.clipboard.writeText(item.registration_url!);setMessage(t("copied"))}catch{setMessage(t("copyFailed"))}}}>{t("copyRegistration")}</Button></div>:null}</section>
 {editable&&!item.delivery.handed_over_at?<section className="rounded-card border border-hairline bg-canvas-raised p-6"><h2 className="font-semibold">{t("operatingPlan")}</h2><div className="my-3 grid max-w-sm gap-3"><label className="grid gap-1 text-sm">{t("stage")}<select className="h-10 rounded-control border border-hairline bg-canvas-raised px-3" value={phase} disabled={busy} onChange={event=>setPhase(event.target.value)}>{["configuring","awaiting_acceptance","paused"].map(value=><option key={value} value={value}>{t(`phase.${value}`)}</option>)}</select></label><label className="grid gap-1 text-sm">{t("salesMode")}<select className="h-10 rounded-control border border-hairline bg-canvas-raised px-3" value={mode} onChange={e=>setMode(e.target.value)}><option value="offline">{t("offline")}</option><option value="online">{t("online")}</option></select></label><label className="flex gap-2 text-sm"><input type="checkbox" checked={plans} onChange={e=>setPlans(e.target.checked)}/>{t("sellPlans")}</label></div><ConfirmButton title={t("savePlan")} description={t("planConfirm",{mode:t(mode)})} onConfirm={()=>mutate("",{expected_version:item.delivery.version,sales_mode:mode,sell_plans:plans,phase},"PATCH")}>{t("savePlan")}</ConfirmButton></section>:null}
 {!own&&canViewAdminHref("/admin/channels",viewer)?<><div id="models"><ChannelModelsPanel key={`${viewer.userId}:${item.channel.id}`} channelID={item.channel.id}/></div>{canWrite("channels.quota",viewer)?<div id="quota"><ChannelQuotaPanel channelID={item.channel.id} channelType="C"/></div>:null}{editable?<div id="admins"><ChannelAdminsPanel channelID={item.channel.id} code={item.brand.name}/></div>:null}</>:null}
 {editable&&!item.delivery.handed_over_at?<section className="rounded-card border border-hairline bg-canvas-raised p-6"><h2 className="font-semibold">{t("handoff")}</h2><select aria-label={t("receiver")} className="my-3 h-10 max-w-md rounded-control border border-hairline bg-canvas-raised px-3" value={receiver} onChange={e=>setReceiver(e.target.value)}><option value="">{t("chooseReceiver")}</option>{item.administrators.filter(a=>a.login_at).map(admin=><option key={admin.user_id} value={admin.user_id}>{admin.email}</option>)}</select><p className="mb-3 text-sm text-ink-secondary">{t("handoffHint")}</p><ConfirmButton disabled={busy||!item.ready||!receiver||item.delivery.phase==="paused"} title={t("handoff")} description={t("handoffConfirm",{email:item.administrators.find(a=>a.user_id===receiver)?.email||"—",name:item.brand.name})} onConfirm={()=>mutate("/handoff",{expected_version:item.delivery.version,receiver_user_id:receiver})}>{t("handoff")}</ConfirmButton></section>:null}
 {item.request_evidence?<p className="break-all text-sm">{t("requestProof",{id:item.request_evidence.request_id,model:item.request_evidence.public_model_id})}</p>:null}
 </div>;
}
