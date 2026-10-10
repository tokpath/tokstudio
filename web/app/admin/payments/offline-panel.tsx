"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { ConfirmDialog } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useViewer } from "@/components/rbac/viewer-context";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { parseUsdToMinor, formatUsdMinor } from "@/lib/money";
import { beginOperation, finishOperation, loadOperation, type SavedOperation } from "@/lib/stable-operation";
import { safeReturnHref } from "@/lib/return-context";

type Customer = { id: string; email: string; display_name: string; channel_code: string };
type Preview = { payee_channel_org_id: string; channel_org_id:string; issue_ratio_bps:number; pool_before_minor?:number;pool_debit_minor:number;pool_after_minor?:number };
type Receipt = { user_id: string; amount_minor: number; credit_minor: number; currency: string; reference: string;note:string;occurred_at:string;expected_issue_ratio_bps:number;customer:Customer;preview:Preview };
const localTime=()=>new Date(Date.now()-new Date().getTimezoneOffset()*60000).toISOString().slice(0,16);
export function OfflineReceiptPanel({ basePath, onRecorded }: { basePath: string; onRecorded: () => void }) {
 const t=useTranslations("paymentTasks");const viewer=useViewer();const params=useSearchParams();
 const fixedCustomer=params.get("customer_id")||"";const returnTo=params.get("return_to");
 const storageKey=`offline-receipt:${basePath}:${viewer.userId||"loading"}:${fixedCustomer||"draft"}`;
 const [expanded,setExpanded]=useState(Boolean(fixedCustomer));const [search,setSearch]=useState("");const [customers,setCustomers]=useState<Customer[]>([]);const [customer,setCustomer]=useState<Customer|null>(null);
 const [amount,setAmount]=useState("");const [credit,setCredit]=useState("");const [currency,setCurrency]=useState("CNY");const [reference,setReference]=useState("");const [note,setNote]=useState("");const [when,setWhen]=useState(localTime);
 const [error,setError]=useState("");const [message,setMessage]=useState("");const [searching,setSearching]=useState(false);const [reviewing,setReviewing]=useState(false);const [receipt,setReceipt]=useState<Receipt|null>(null);const [operation,setOperation]=useState<SavedOperation<Receipt>|null>(null);const [conflict,setConflict]=useState(false);const [orderID,setOrderID]=useState("");
 const generation=useRef(0);const submitting=useRef(false);
 useEffect(()=>{
  const saved=loadOperation<Receipt>(storageKey);setOperation(saved);setConflict(false);setReceipt(null);setCustomer(saved?.payload.customer||null);setCustomers([]);setAmount(saved?String(saved.payload.amount_minor/(saved.payload.currency==="USD"?1_000_000:100)):"");setCredit(saved?String(saved.payload.credit_minor/1_000_000):"");setCurrency(saved?.payload.currency||"CNY");setReference(saved?.payload.reference||"");setNote(saved?.payload.note||"");setWhen(saved?new Date(new Date(saved.payload.occurred_at).getTime()-new Date(saved.payload.occurred_at).getTimezoneOffset()*60000).toISOString().slice(0,16):localTime());setError("");setMessage(saved?t("pending"):"");setOrderID("");setExpanded(Boolean(saved||fixedCustomer));
  if(!saved&&fixedCustomer)void findCustomers(fixedCustomer,true);
  return()=>{generation.current++};
 },[storageKey]);
 async function findCustomers(value=search,fixed=false){
  const version=++generation.current;setError("");setCustomer(null);setCustomers([]);setSearching(true);
  try{const response=await fetch(`${apiBase}${basePath}/recipients?q=${encodeURIComponent(value.trim())}`,{credentials:"include"});const body=await response.json();if(version!==generation.current)return;if(!response.ok||!Array.isArray(body.items))throw new Error(body.error?.message||t("customerFailed"));const items=body.items as Customer[];setCustomers(items);if(fixed){const match=items.find(item=>item.id===value);if(!match){setError(t("customerUnavailable"));return}setCustomer(match)}else setMessage(items.length?"":t("noCustomer"))}catch(e){if(version===generation.current)setError(e instanceof Error?e.message:t("customerFailed"))}finally{if(version===generation.current)setSearching(false)}
 }
 async function review(){
  setError("");if(operation){setReceipt(operation.payload);return}
  const creditMinor=parseUsdToMinor(credit);const payMinor=currency==="USD"?parseUsdToMinor(amount):/^\d+(\.\d{1,2})?$/.test(amount.trim())?Math.round(Number(amount)*100):null;const occurred=new Date(when);
  if(!customer||!creditMinor||creditMinor<=0||!payMinor||payMinor<=0||!Number.isSafeInteger(payMinor)||!when||!Number.isFinite(occurred.getTime())||occurred.getTime()>Date.now()+300000||new TextEncoder().encode(reference.trim()).length>200){setError(t("fillReceipt"));return}
  const version=++generation.current;setReviewing(true);
  try{const response=await fetch(`${apiBase}${basePath}/offline/preview?user_id=${encodeURIComponent(customer.id)}&credit_minor=${creditMinor}`,{credentials:"include"});const body=await response.json();if(version!==generation.current)return;if(!response.ok||!body.item){setError(body.error?.message||t("previewFailed"));return}const preview=body.item as Preview;setReceipt({user_id:customer.id,amount_minor:payMinor,credit_minor:creditMinor,currency,reference:reference.trim(),note:note.trim(),occurred_at:occurred.toISOString(),expected_issue_ratio_bps:preview.issue_ratio_bps,customer,preview})}catch{if(version===generation.current)setError(t("previewFailed"))}finally{if(version===generation.current)setReviewing(false)}
 }
 function completed(id:string,status="paid"){finishOperation(storageKey);setOperation(null);setConflict(false);setOrderID(id);setAmount("");setCredit("");setReference("");setNote("");setWhen(localTime());setMessage(t(status==="refunded"?"originalRefunded":status==="refunding"?"originalRefunding":"recorded",{id}));onRecorded()}
 async function record(){
  if(!receipt||submitting.current)return false;submitting.current=true;setError("");let pending=operation;
  try{pending=beginOperation(storageKey,receipt);setOperation(pending);const {customer:_,preview:__,...payload}=pending.payload;const response=await fetch(`${apiBase}${basePath}/offline`,{method:"POST",credentials:"include",headers:confirmHeaders,body:JSON.stringify({...payload,operation_id:pending.id})});const body=await response.json();
   if(!response.ok){if(body.error?.code==="operation_conflict")setConflict(true);if((response.status>=400&&response.status<500&&![408,409,429].includes(response.status))||body.error?.code==="preview_changed"||body.error?.code==="insufficient_quota"){finishOperation(storageKey);setOperation(null)}if(body.error?.code==="external_transaction_conflict"&&body.error?.param?.order_id){setOrderID(body.error.param.order_id);finishOperation(storageKey);setOperation(null)}setError(body.error?.message||t("unknown"));return false}
   if(!body.item?.id||!body.item.fulfilled_at)throw new Error();completed(body.item.id,body.item.status);return true
  }catch{setError(pending?t("unknown"):t("storageFailed"));return false}finally{submitting.current=false}
 }
 async function lookup(){if(!operation)return;try{const response=await fetch(`${apiBase}${basePath}/offline/operations/${encodeURIComponent(operation.id)}`,{credentials:"include"});const body=await response.json();if(!response.ok){setMessage(t("lookupFailed"));return}if(body.operation_status==="recorded"&&body.item?.id){completed(body.item.id,body.item.status);setReceipt(null)}else if(conflict&&body.operation_status==="not_found"){finishOperation(storageKey);setOperation(null);setConflict(false);setMessage(t("confirmedConflict"))}else setMessage(t("notFoundYet"))}catch{setMessage(t("lookupFailed"))}}
 const chosen=receipt||operation?.payload;const pool=chosen?.preview;
 const description=chosen?[`${chosen.customer.display_name} · ${chosen.customer.email}`,t("receiptSummary",{amount:String(chosen.amount_minor/(chosen.currency==="USD"?1_000_000:100)),currency:chosen.currency,credit:formatUsdMinor(chosen.credit_minor),time:new Date(chosen.occurred_at).toLocaleString(undefined,{timeZone:"Asia/Shanghai"})}),pool?.pool_before_minor!==undefined?t("poolSummary",{before:formatUsdMinor(pool.pool_before_minor),debit:formatUsdMinor(pool.pool_debit_minor),after:formatUsdMinor(pool.pool_after_minor)}):t("platformFunds"),chosen.reference?t("referenceValue",{value:chosen.reference}):"",t("actualReceived")].filter(Boolean).join("\n"):"";
 return <div className="space-y-3">
  <Button variant="outline" onClick={()=>setExpanded(value=>!value)}>{expanded?t("collapse"):t("offline")}</Button>
  {expanded?<section aria-label={t("offline")} className="space-y-4 rounded-control border border-hairline p-4">
   <p className="text-sm text-ink-secondary">{t("offlineHint")}</p>
   <fieldset disabled={Boolean(operation)||reviewing} className="space-y-3">
    {!fixedCustomer?<form className="flex gap-2" onSubmit={e=>{e.preventDefault();void findCustomers()}}><Input aria-label={t("findCustomer")} placeholder={t("searchCustomer")} value={search} onChange={e=>{generation.current++;setSearch(e.target.value);setCustomer(null);setCustomers([]);setAmount("");setCredit("");setSearching(false);setReviewing(false)}} maxLength={200}/><Button type="submit" disabled={search.trim().length<2||searching}>{t("find")}</Button></form>:null}
    {searching?<p role="status">{t("loadingCustomer")}</p>:null}
    <ul className="space-y-2">{customers.map(item=><li key={item.id}><button disabled={Boolean(fixedCustomer)} type="button" aria-pressed={customer?.id===item.id} className={`w-full rounded-control border p-3 text-left text-sm ${customer?.id===item.id?"border-brand-emphasis bg-brand-soft":"border-hairline"}`} onClick={()=>{setCustomer(item);setAmount("");setCredit("")}}>{item.display_name} · {item.email} <span className="text-ink-secondary">{item.channel_code||t("platformDirect")}</span></button></li>)}</ul>
    {customer&&!customers.length?<p className="break-all text-sm">{customer.display_name} · {customer.email}</p>:null}
    <div className="grid gap-3 md:grid-cols-3">
     <label className="grid gap-1 text-sm">{t("currency")}<select className="h-10 rounded-control border border-hairline bg-canvas px-3" value={currency} onChange={e=>setCurrency(e.target.value)}><option>CNY</option><option>USD</option></select></label>
     <label className="grid gap-1 text-sm">{t("receivedAmount")}<Input inputMode="decimal" value={amount} onChange={e=>setAmount(e.target.value)}/></label>
     <label className="grid gap-1 text-sm">{t("credit")}<Input inputMode="decimal" value={credit} onChange={e=>setCredit(e.target.value)}/></label>
     <label className="grid gap-1 text-sm">{t("occurredAt")}<Input type="datetime-local" value={when} onChange={e=>setWhen(e.target.value)}/></label>
     <label className="grid gap-1 text-sm">{t("reference")}<Input maxLength={200} value={reference} onChange={e=>setReference(e.target.value)}/></label>
     <label className="grid gap-1 text-sm">{t("note")}<Input maxLength={2000} value={note} onChange={e=>setNote(e.target.value)}/></label>
    </div>
   </fieldset>
   {operation?<p role="status" className="break-all text-sm text-hold">{t("original",{id:operation.id,email:operation.payload.customer.email,amount:formatUsdMinor(operation.payload.credit_minor)})}</p>:null}
   <Button disabled={!customer||reviewing||viewer.loading||!viewer.userId} onClick={()=>void review()}>{operation?t("retryOriginal"):reviewing?t("checking"):t("review")}</Button>
   {operation?<Button variant="outline" className="ml-2" onClick={()=>void lookup()}>{t("lookup")}</Button>:null}
  </section>:null}
  {message?<p role="status" aria-live="polite" className="text-sm">{message}</p>:null}
  {orderID?<Link className="text-brand-emphasis underline" href={`${basePath==="/admin/payments"?"/admin/payments":"/channel/payments/orders"}/${encodeURIComponent(orderID)}`}>{t("openOrder")}</Link>:null}
  {fixedCustomer&&returnTo?<Link className="ml-3 text-brand-emphasis underline" href={safeReturnHref(returnTo,basePath==="/admin/payments"?"/admin/users":"/channel/users")}>{t("backCustomer")}</Link>:null}
  {error&&!receipt?<p role="alert" className="text-danger">{error}</p>:null}
  <ConfirmDialog open={Boolean(receipt)} onOpenChange={open=>{if(!open)setReceipt(null)}} title={t("confirmReceipt")} description={description} error={error} onConfirm={record}/>
 </div>
}
