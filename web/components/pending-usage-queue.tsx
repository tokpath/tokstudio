"use client";
import Link from "next/link";
import { useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { AdminListPanel } from "@/app/admin/list-panel";
import { useBrand } from "@/components/brand-context";
import { useViewer } from "@/components/rbac/viewer-context";
import { IfCan } from "@/components/rbac/if-can";
import { ConfirmButton } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { appendReturnContext } from "@/lib/return-context";
import { beginOperation,finishOperation,loadOperation,type SavedOperation } from "@/lib/stable-operation";
import { gapKey,type UsageGap } from "@/lib/reconciliation";
import { formatUsdMinor } from "@/lib/money";

type Resolution = {key:string;status:string;error?:string};
type Payload = {ids:string[]};
export function PendingUsageQueue({surface,channelID}:{surface:"admin"|"channel";channelID?:string}) {
 const t=useTranslations("usageWorkflow"),viewer=useViewer(),brand=useBrand(),client=useQueryClient();
 const base=`/${surface}`,operationKey=`usage-release:${viewer.userId}:${brand?.id||""}:${surface}:${channelID||"brand"}`;
 const [status,setStatus]=useState("pending_reconciliation"),[from,setFrom]=useState(""),[to,setTo]=useState(""),[picked,setPicked]=useState<string[]>([]);
 const [selected,setSelected]=useState<UsageGap|null>(null),[detailState,setDetailState]=useState(""),[message,setMessage]=useState(""),[results,setResults]=useState<Resolution[]>([]);
 const [operation,setOperation]=useState<SavedOperation<Payload>|null>(null),[ready,setReady]=useState(false);
 const sequence=useRef(0);
 useEffect(()=>{setOperation(loadOperation<Payload>(operationKey));setPicked([]);setSelected(null);setResults([]);setMessage("");setReady(true);return()=>{sequence.current++}},[operationKey]);
 const params=useMemo(()=>{const p=new URLSearchParams({status,time_zone:"Asia/Shanghai"});if(from)p.set("from",from);if(to)p.set("to",to);if(channelID)p.set("channel_id",channelID);return p;},[status,from,to,channelID]);
 const scope=new URLSearchParams();if(channelID)scope.set("channel_id",channelID);
 const scopeSuffix=scope.size?`?${scope}`:"";
 function filter(update:()=>void){if(operation)return;sequence.current++;setPicked([]);setSelected(null);setDetailState("");setResults([]);setMessage("");update()}
 async function open(row:UsageGap){const seq=++sequence.current;setSelected(row);setDetailState("loading");try{const response=await fetch(`${apiBase}${base}/usage/pending/${encodeURIComponent(gapKey(row))}${scopeSuffix}`,{credentials:"include"});const body=await response.json();if(seq!==sequence.current)return;if(!response.ok||!body.item){setDetailState("error");return}setSelected(body.item);setDetailState("ready")}catch{if(seq===sequence.current)setDetailState("error")}}
 async function resolve(ids:string[]) {
  setMessage("");setResults([]);
  let saved=operation;
  try{saved=saved||beginOperation(operationKey,{ids:[...new Set(ids.filter(Boolean))]});setOperation(saved)}catch{setMessage(t("storageFailed"));return false}
  if(!saved.payload.ids.length){setMessage(t("needSelection"));return false}
  try{
   const response=await fetch(`${apiBase}${base}/usage/pending/resolve${scopeSuffix}`,{method:"POST",credentials:"include",headers:{...confirmHeaders,"Idempotency-Key":saved.id},body:JSON.stringify(saved.payload)});
   const body=await response.json();
   if(!response.ok){setMessage(body.error?.message||t("readFailed"));if(response.status<500){finishOperation(operationKey);setOperation(null)}return false}
   const rows:Resolution[]=body.item?.results||[];
   if(!rows.length){setMessage(t("unknownOutcome"));return false}
   setResults(rows);const failed=rows.filter(r=>r.status!=="resolved");setPicked(failed.map(r=>r.key));setMessage(t("resolvedCounts",{resolved:rows.filter(r=>r.status==="resolved").length,failed:failed.length}));
   if(!failed.some(r=>r.error==="read_error")){finishOperation(operationKey);setOperation(null)}
   await client.invalidateQueries({predicate:q=>q.queryKey.some(k=>typeof k==="string"&&(k.includes("usage")||k.includes("request")||k.includes("dashboard")||k.includes("balance")))});
   if(selected)void open(selected);return failed.length===0;
  }catch{setMessage(t("unknownOutcome"));return false}
 }
 const activeIDs=operation?.payload.ids||picked;
 const description=`${t("releaseHelp")}\n${activeIDs.join("\n")}`;
 const detailHref=(id:string)=>appendReturnContext(`${base}/usage/requests/${encodeURIComponent(id)}${scopeSuffix}`,typeof window==="undefined"?`${base}/reconciliation`:window.location.pathname+window.location.search);
 if(!ready)return <p role="status">{t("loading")}</p>;
 return <div className="flex flex-col gap-5"><h1 className="text-2xl font-semibold">{t("pendingQueue")}</h1>
  <section className="rounded-card border border-hairline bg-canvas-raised p-5"><p className="text-sm">{t("queueHelp")}</p><div className="mt-4 flex flex-wrap items-end gap-3">
   <label className="grid gap-1 text-sm">{t("billing")}<select className="h-10 rounded-control border border-hairline bg-canvas px-2" disabled={!!operation} value={status} onChange={e=>filter(()=>setStatus(e.target.value))}>{["pending_reconciliation","voided","all"].map(s=><option key={s} value={s}>{t(s)}</option>)}</select></label>
   {(["from","to"] as const).map(k=><label className="grid gap-1 text-sm" key={k}>{t(k)}<Input disabled={!!operation} type="date" value={k==="from"?from:to} onChange={e=>filter(()=>k==="from"?setFrom(e.target.value):setTo(e.target.value))}/></label>)}
   {surface==="admin"?<IfCan action="usage.resolve"><ConfirmButton title={t("release")} description={description} validate={()=>!!activeIDs.length} onConfirm={()=>resolve(activeIDs)} disabled={!activeIDs.length}>{operation?t("retryOriginal"):t("releaseSelected")}</ConfirmButton></IfCan>:<p className="text-sm">{t("platformReview")}</p>}
  </div><p className="mt-2 text-xs text-ink-mute">Asia/Shanghai</p>
  {operation?<div role="alert" className="mt-3 text-sm"><p>{t("originalOperation")}</p><ul className="mt-2 font-mono">{operation.payload.ids.map(id=><li key={id}>{id}</li>)}</ul></div>:null}
  {message?<p role="status" className="mt-3 text-sm">{message}</p>:null}
  {results.length?<ul className="mt-3 space-y-1 text-sm">{results.map(r=><li key={r.key}><span className="font-mono">{r.key}</span> · {t(r.status==="resolved"?"voided":r.error==="already_charged"?"alreadyCharged":r.error==="not_found"?"missing":"readFailed")}</li>)}</ul>:null}</section>
  <AdminListPanel<UsageGap> key={params.toString()} path={`${base}/usage/pending?${params}`} title={t("pendingQueue")} emptyTitle={t("emptyPending")} emptyDetail={t("adjust")} onRowSelect={r=>void open(r)} rowSelected={r=>gapKey(r)===gapKey(selected||{})} columns={[
   {id:"pick",header:t("select"),cell:({row})=><input type="checkbox" aria-label={`${t("select")} ${gapKey(row.original)}`} disabled={!!operation||row.original.state!=="pending_reconciliation"} checked={picked.includes(gapKey(row.original))} onClick={e=>e.stopPropagation()} onChange={e=>{const id=gapKey(row.original);setPicked(v=>e.target.checked?[...new Set([...v,id])]:v.filter(k=>k!==id))}}/>},
   {accessorKey:"occurred_at",header:t("time"),cell:({row})=>new Date(row.original.occurred_at||"").toLocaleString(undefined,{timeZone:"Asia/Shanghai"})},
   {accessorKey:"request_id",header:t("request")},{accessorKey:"public_model_id",header:t("model")},{accessorKey:"reserved_minor",header:t("reserved"),cell:({row})=>`${formatUsdMinor(row.original.reserved_minor)} USD`},{accessorKey:"state",header:t("billing"),cell:({row})=>t(row.original.state==="voided"?"voided":"pending_reconciliation")}
  ]}/>
  {selected?<section className="rounded-card border border-hairline bg-canvas-raised p-5" data-testid="usage-gap-detail"><h2 className="font-semibold">{t("detail")}</h2>{detailState==="loading"?<p role="status">{t("loading")}</p>:detailState==="error"?<div role="alert"><p>{t("readFailed")}</p><Button variant="outline" onClick={()=>void open(selected)}>{t("retry")}</Button></div>:<><p className="mt-3 break-all text-sm">{selected.request_id} · {selected.public_model_id}</p><p className="mt-2 text-sm">{t("reserved")} {formatUsdMinor(selected.reserved_minor)} USD · {t(selected.state==="voided"?"voided":"pending_reconciliation")}</p>{selected.request_id?<Link className="mt-3 inline-block text-sm text-brand" href={detailHref(selected.request_id)}>{t("openRequest")}</Link>:null}</>}</section>:null}
 </div>;
}
