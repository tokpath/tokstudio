"use client";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { useViewer } from "@/components/rbac/viewer-context";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { ConfirmButton } from "@/components/confirm-button";
import { confirmHeaders } from "@/lib/confirm";
import { apiBase } from "@/lib/api";
import { readAllPages } from "@/lib/api-pages";
import { apiClient } from "@/lib/client";
import { beginOperation, finishOperation, loadOperation, type SavedOperation } from "@/lib/stable-operation";

type Brand = {id:string;name:string;primary_domain:string;api_domain:string;admin_domain:string};
type Payload = {brand_id:string;brand?:{name:string;primary_domain:string;api_domain:string;admin_domain:string};sales_mode:string;sell_plans:boolean};
export function OEMCreateDialog({open,onOpenChange}:{open:boolean;onOpenChange:(open:boolean)=>void}) {
 const t=useTranslations("oemDelivery");const router=useRouter();const viewer=useViewer();
 const storageKey=`oem-create:${viewer.userId||"loading"}`;
 const [brandID,setBrandID]=useState("");const [name,setName]=useState("");const [primary,setPrimary]=useState("");const [api,setAPI]=useState("");const [admin,setAdmin]=useState("");const [mode,setMode]=useState("offline");const [plans,setPlans]=useState(false);const [message,setMessage]=useState("");const [operation,setOperation]=useState<SavedOperation<Payload>|null>(null);
 const brands=useQuery({queryKey:[viewer.userId,"/admin/brands"],queryFn:()=>readAllPages<Brand>("/admin/brands"),enabled:open});
 const channels=useQuery({queryKey:[viewer.userId,"oem-create-owners"],queryFn:()=>readAllPages<{brand_id:string;type:string}>("/admin/channels"),enabled:open});
 useEffect(()=>{const saved=loadOperation<Payload>(storageKey);setOperation(saved);if(saved){setBrandID(saved.payload.brand_id);setName(saved.payload.brand?.name||"");setPrimary(saved.payload.brand?.primary_domain||"");setAPI(saved.payload.brand?.api_domain||"");setAdmin(saved.payload.brand?.admin_domain||"");setMode(saved.payload.sales_mode);setPlans(saved.payload.sell_plans);setMessage(t("pendingCreate"));}},[storageKey,t]);
 const available=(brands.data?.items||[]).filter(brand=>!channels.data?.items.some(item=>["A","C"].includes(item.type)&&item.brand_id===brand.id));
 function payload():Payload{return{brand_id:brandID,...(!brandID?{brand:{name:name.trim(),primary_domain:primary.trim().toLowerCase(),api_domain:api.trim().toLowerCase(),admin_domain:admin.trim().toLowerCase()}}:{}),sales_mode:mode,sell_plans:plans}}
 function validate(){if(operation)return true;if(!viewer.userId||!brands.data||!channels.data){setMessage(t("loadFailed"));return false}if(!brandID&&(!name.trim()||![primary,api,admin].every(host=>/^[a-zA-Z0-9.-]+$/.test(host.trim())&&host.includes(".")))){setMessage(t("fillBrand"));return false}return true}
 async function create(){if(!validate())return false;let pending=operation;try{pending=beginOperation(storageKey,payload());setOperation(pending);const response=await fetch(`${apiBase}/admin/oem-deliveries`,{method:"POST",credentials:"include",headers:confirmHeaders,body:JSON.stringify({...pending.payload,operation_id:pending.id})});const body=await response.json();if(!response.ok||!body.item){if(response.status>=400&&response.status<500&&![408,409,429].includes(response.status)){finishOperation(storageKey);setOperation(null)}setMessage(body.error?.message||t("createFailed"));return false}finishOperation(storageKey);setOperation(null);onOpenChange(false);router.push(`/admin/oem-deliveries/${encodeURIComponent(body.item.channel_org_id)}`);return true}catch{setMessage(t("pendingCreate"));return false}}
 async function lookup(){if(!operation)return;try{const response=await fetch(`${apiBase}/admin/oem-deliveries?operation_id=${encodeURIComponent(operation.id)}`,{credentials:"include"});const body=await response.json();if(response.ok&&body.item){finishOperation(storageKey);setOperation(null);onOpenChange(false);router.push(`/admin/oem-deliveries/${encodeURIComponent(body.item.channel_org_id)}`)}else if(response.status===404){setMessage(t("pendingCreate"))}else setMessage(t("pendingCreate"))}catch{setMessage(t("pendingCreate"))}}
 return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent><DialogHeader><DialogTitle>{t("createTitle")}</DialogTitle><DialogDescription>{t("createHint")}</DialogDescription></DialogHeader>
 <fieldset disabled={Boolean(operation)} className="grid gap-3">
 <label className="grid gap-1 text-sm">{t("brand")}<select className="h-10 rounded-control border border-hairline bg-canvas-raised px-3" value={brandID} onChange={e=>setBrandID(e.target.value)}><option value="">{t("newBrand")}</option>{available.map(brand=><option key={brand.id} value={brand.id}>{brand.name} · {brand.primary_domain}</option>)}</select></label>
 {!brandID?([['name',name,setName],['primary',primary,setPrimary],['api',api,setAPI],['admin',admin,setAdmin]] as const).map(([key,value,set])=><label key={key} className="grid gap-1 text-sm">{t(key)}<Input aria-label={t(key)} value={value} onChange={e=>set(e.target.value)}/></label>):null}
 <label className="grid gap-1 text-sm">{t("salesMode")}<select className="h-10 rounded-control border border-hairline bg-canvas-raised px-3" value={mode} onChange={e=>setMode(e.target.value)}><option value="offline">{t("offline")}</option><option value="online">{t("online")}</option></select></label>
 <label className="flex gap-2 text-sm"><input type="checkbox" checked={plans} onChange={e=>setPlans(e.target.checked)}/>{t("sellPlans")}</label>
 </fieldset>{operation?<div><p className="break-all text-sm text-hold">{t("originalOperation",{id:operation.id})}</p><Button variant="outline" onClick={()=>void lookup()}>{t("lookupOriginal")}</Button></div>:null}
 {brands.isError||channels.isError?<p role="alert">{t("loadFailed")}</p>:null}
 <ConfirmButton disabled={viewer.loading||!viewer.userId} title={t("createTitle")} description={t("createConfirm",{name:operation?.payload.brand?.name||available.find(x=>x.id===brandID)?.name||name,mode:t(mode)})} validate={validate} onConfirm={create}>{operation?t("retryOriginal"):t("createTitle")}</ConfirmButton>
 {message?<p role="status" className="text-sm">{message}</p>:null}
 </DialogContent></Dialog>;
}
