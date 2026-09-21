"use client";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { IfCan } from "@/components/rbac/if-can";
import { apiBase } from "@/lib/api";
export function ProbeCell({ id }: { id: string }) {
 const client=useQueryClient();const [result,setResult]=useState("");const [pending,setPending]=useState(false);const [failed,setFailed]=useState(false);
 return <div className="flex flex-wrap items-center gap-2"><IfCan action="providers.health"><Button size="sm" variant="outline" disabled={pending} onClick={async event=>{event.stopPropagation();setPending(true);setResult("");setFailed(false);try{const r=await fetch(`${apiBase}/admin/providers/${encodeURIComponent(id)}/health-check`,{method:"POST",credentials:"include",headers:{"Content-Type":"application/json"},body:"{}"});const body=await r.json();setFailed(!r.ok);setResult(r.ok?`沙箱结果：${({available:"可用",unavailable:"不可用",degraded:"降级"} as Record<string,string>)[body.health]||"未知"}，不代表真实上游连通性。`:body.error?.message||"探测未完成，请重试。");if(r.ok)void client.invalidateQueries({queryKey:["/admin/providers"]});}catch{setFailed(true);setResult("连接中断，未获得探测结果，请重试。");}finally{setPending(false);}}}>{pending?"探测中…":"探测"}</Button></IfCan>{result&&<span role={failed?"alert":"status"} className="max-w-xs whitespace-normal text-xs text-ink-secondary">{result}</span>}</div>;
}
