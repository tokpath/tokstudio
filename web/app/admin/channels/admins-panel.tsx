"use client";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ConfirmDialog } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
export function ChannelAdminsPanel({ channelID, code }: { channelID: string; code: string }) {
 const [email,setEmail]=useState(""); const [reason,setReason]=useState(""); const [error,setError]=useState(""); const [message,setMessage]=useState("");
 const [operation,setOperation]=useState<{email:string;reason:string;enabled:boolean}|null>(null);
 const query=useQuery({queryKey:["channel-admins",channelID],queryFn:async()=>{const r=await fetch(`${apiBase}/admin/channels/${encodeURIComponent(channelID)}/admins`,{credentials:"include"});if(!r.ok)throw new Error("load");return r.json() as Promise<{items:{user_id:string;email:string}[]}>;}});
 function prepare(enabled:boolean){setError("");setMessage("");setOperation({email:email.trim(),reason:reason.trim(),enabled});}
 async function submit(){if(!operation)return false;setError("");try{const r=await fetch(`${apiBase}/admin/channels/${encodeURIComponent(channelID)}/admins`,{method:"POST",credentials:"include",headers:confirmHeaders,body:JSON.stringify(operation)});const b=await r.json();if(!r.ok){setError(b.error?.message||"保存失败，请重试。");return false;}setMessage(b.changed?"渠道管理权限已更新，下一次请求即生效。":"权限已是目标状态，没有重复修改。");setOperation(null);void query.refetch();return true;}catch{setError(confirmNetworkUnavailable);return false;}}
 return <section className="rounded-card border border-hairline bg-canvas-raised p-6"><h2 className="text-lg font-semibold">渠道管理员交接</h2><p className="mt-2 text-sm text-ink-secondary">先让接手人通过该渠道推广链接注册，再按邮箱授权。只允许本渠道已有用户，不会更改用户归属或授予平台管理权限。撤销后保留其个人账户和历史账务。</p>
 {query.isLoading?<p role="status">正在读取管理员…</p>:query.isError?<p role="alert">读取失败，请刷新重试。</p>:<ul className="my-3">{query.data?.items?.length?query.data.items.map(x=><li key={x.user_id}>{x.email}</li>):<li>尚未配置管理员，请完成交接后再交付渠道。</li>}</ul>}
 <div className="my-3 flex flex-wrap gap-3"><Input aria-label="渠道管理员邮箱" placeholder="接手人的注册邮箱" value={email} onChange={e=>setEmail(e.target.value)}/><Input aria-label="权限变更原因" placeholder="授权或撤销的原因" value={reason} onChange={e=>setReason(e.target.value)}/><Button disabled={!email.trim()||!reason.trim()} onClick={()=>prepare(true)}>授权渠道管理</Button><Button variant="outline" disabled={!email.trim()||!reason.trim()} onClick={()=>prepare(false)}>撤销渠道管理</Button></div>
 {message?<p role="status">{message}</p>:null}<ConfirmDialog open={!!operation} onOpenChange={open=>{if(!open)setOperation(null);}} title={operation?.enabled?"确认授权渠道管理员":"确认撤销渠道管理员"} description={`渠道 ${code}；用户 ${operation?.email}；原因 ${operation?.reason}。${operation?.enabled?"可管理本渠道用户、收款配置和经营数据，请核对接手人。":"将立即失去该渠道管理权限，个人账户保留。"}`} error={error} onConfirm={submit}/></section>;
}
