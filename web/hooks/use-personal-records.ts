"use client";
import { useEffect, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useBrand } from "@/components/brand-context";
import { useViewer } from "@/components/rbac/viewer-context";
import { useListResource } from "./use-list-resource";
import { initialListSnapshot } from "@/lib/list-resource";
import { apiBase } from "@/lib/api";
import { subscribeWalletChanged } from "@/lib/wallet-events";
export type PersonalPage<T> = { items: T[]; total: number; next_cursor: string };
export function usePersonalRecords<T>(kind: "orders" | "ledger" | "recoveries") {
  const viewer=useViewer();const brand=useBrand();const search=useSearchParams();const pathname=usePathname();const router=useRouter();
  const scope=`${viewer.userId || ""}:${brand?.id || ""}`;
  const scopeParam=`${kind}_scope`;const cursorParam=`${kind}_cursor`;
  const foreign=!!search.get(scopeParam) && search.get(scopeParam)!==scope;
  const cursor=foreign ? "" : search.get(cursorParam) || "";
  const key=`${scope}:${kind}:${cursor}`;const [acceptedKey,setAcceptedKey]=useState("");
  const list=useListResource<PersonalPage<T>>({queryKey:key,enabled:!viewer.loading && !!viewer.userId,onAccepted:()=>setAcceptedKey(key),load:async()=>{
    const response=await fetch(`${apiBase}/v1/me/wallet-records?kind=${kind}&cursor=${encodeURIComponent(cursor)}`,{credentials:"include"});const body=await response.json();
    const valid=Array.isArray(body.items) && Number.isSafeInteger(body.total);
    return {ok:response.ok && valid,status:response.status,items:response.ok && valid ? [body] : [],message:body.error?.message,code:body.error?.code};
  }});
  function href(nextCursor:string){const params=new URLSearchParams(search.toString());if(nextCursor){params.set(cursorParam,nextCursor);params.set(scopeParam,scope);}else{params.delete(cursorParam);params.delete(scopeParam);}return `${pathname}?${params}`;}
  useEffect(()=>{if(foreign)router.replace(href(""));},[foreign,scope]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(()=>subscribeWalletChanged({userId:viewer.userId || "",brandId:brand?.id || ""},()=>void list.reload()),[scope,list.reload]);
  const snapshot=acceptedKey===key ? list.snapshot : initialListSnapshot<PersonalPage<T>>();
  return {...list,snapshot,page:snapshot.items[0],cursor,href};
}
