"use client";
import { useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { useViewer } from "@/components/rbac/viewer-context";
import { canChannelAction, canWrite } from "@/lib/rbac";
import { formatOrderDue,formatOrderCredit } from "@/lib/checkout";
import { AdminListPanel } from "../list-panel";
import { OfflineReceiptPanel } from "./offline-panel";
import type { PaymentOrder } from "./order-detail";

type Row=PaymentOrder & {user_email?:string;user_name?:string;channel_code?:string};
export function PaymentOrdersPanel({oem=false}:{oem?:boolean}){
 const t=useTranslations("paymentTasks");const viewer=useViewer();const queryClient=useQueryClient();const params=useSearchParams();const [status,setStatus]=useState(params.get("status")||"");
 const listPath=oem?"/channel/payments/orders":"/admin/payments";const writable=oem?canChannelAction("finance",viewer):canWrite("payments.write",viewer);
 useEffect(()=>{setStatus(params.get("status")||"")},[params]);
 const label=(value:string)=>["pending","paid","refunding","refunded","failed","expired","wallet","subscription","renewal","manual","stripe","alipay","wechat"].includes(value)?t(value):value;
 return <div className="space-y-6"><h1 className="text-2xl font-semibold">{t("title")}</h1>
  {writable?<section className="rounded-card border border-hairline bg-canvas-raised p-6"><OfflineReceiptPanel basePath={oem?"/channel/payments":"/admin/payments"} onRecorded={()=>void queryClient.invalidateQueries({predicate:query=>query.queryKey.some(key=>typeof key==="string"&&key.startsWith(listPath))})}/></section>:null}
  <AdminListPanel<Row> key={`${viewer.userId}:${status}`} path={`${listPath}${status?`?status=${encodeURIComponent(status)}`:""}`} title={t("title")} emptyTitle={t("empty")} emptyDetail={t("searchHint")} rowHref={row=>`${listPath}/${encodeURIComponent(row.id)}`}
   actions={<label className="flex items-center gap-2 text-sm">{t("status")}<select className="h-9 rounded-control border border-hairline bg-canvas-raised px-3" value={status} onChange={event=>{setStatus(event.target.value);const url=new URL(window.location.href);if(event.target.value)url.searchParams.set("status",event.target.value);else url.searchParams.delete("status");for(const key of [...url.searchParams.keys()])if(key.endsWith("_cursor"))url.searchParams.delete(key);window.history.replaceState(null,"",url.toString())}}><option value="">{t("allStatus")}</option>{["pending","paid","refunding","refunded","failed","expired"].map(value=><option key={value} value={value}>{t(value)}</option>)}</select></label>}
   columns={[
    {accessorKey:"id",header:t("details"),cell:({row})=><span className="break-all font-mono text-xs">{row.original.id}</span>},
    {id:"customer",header:t("customer"),cell:({row})=><div className="break-all">{row.original.user_name}<p>{row.original.user_email||row.original.user_id}</p><p className="text-ink-secondary">{row.original.channel_code||row.original.channel_org_id||"—"}</p></div>},
    {id:"amount",header:t("amount"),cell:({row})=><div className="tabular-nums">{formatOrderDue(row.original)} {row.original.currency}{row.original.purpose==="wallet"?<p className="text-ink-secondary">{t("credit")}: {formatOrderCredit(row.original)} USD</p>:null}</div>},
    {accessorKey:"purpose",header:t("purpose"),cell:({row})=>label(row.original.purpose)},
    {accessorKey:"adapter",header:t("adapter"),cell:({row})=>label(row.original.adapter)},
    {accessorKey:"status",header:t("status"),cell:({row})=><div>{label(row.original.status)}{row.original.status==="paid"?<p className="text-ink-secondary">{t(row.original.fulfilled_at?"fulfilled":"unfulfilled")}</p>:null}</div>},
    {accessorKey:"created_at",header:t("createdAt"),cell:({row})=>new Date(row.original.created_at).toLocaleString(undefined,{timeZone:"Asia/Shanghai"})}
   ]}/>
 </div>
}
