"use client";
import { useTranslations } from "next-intl";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { IfCan } from "@/components/rbac/if-can";
import { apiBase } from "@/lib/api";
export function ProbeCell({ id }: { id: string }) {
  const tUI = useTranslations();
 const client=useQueryClient();const [result,setResult]=useState("");const [pending,setPending]=useState(false);const [failed,setFailed]=useState(false);
 return <div className="flex flex-wrap items-center gap-2"><IfCan action="providers.health"><Button size="sm" variant="outline" disabled={pending} onClick={async event=>{event.stopPropagation();setPending(true);setResult("");setFailed(false);try{const r=await fetch(`${apiBase}/admin/providers/${encodeURIComponent(id)}/health-check`,{method:"POST",credentials:"include",headers:{"Content-Type":"application/json"},body:"{}"});const body=await r.json();setFailed(!r.ok);setResult(r.ok?tUI("consoleCopy.sandboxResultThisDoesNotConfirmRealUpstream", { value0: String(({available:tUI("consoleCopy.available"),unavailable:tUI("consoleCopy.unavailable"),degraded:tUI("consoleCopy.degraded")} as Record<string,string>)[body.health]||tUI("consoleCopy.unknown")) }):body.error?.message||tUI("consoleCopy.probeIncompletePleaseRetry"));if(r.ok)void client.invalidateQueries({queryKey:["/admin/providers"]});}catch{setFailed(true);setResult(tUI("consoleCopy.connectionInterruptedNoProbeResultWasReceivedPlease"));}finally{setPending(false);}}}>{pending?tUI("consoleCopy.probing"):tUI("consoleCopy.probe")}</Button></IfCan>{result&&<span role={failed?"alert":"status"} className="max-w-xs whitespace-normal text-xs text-ink-secondary">{result}</span>}</div>;
}
