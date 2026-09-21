"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ConfirmDialog } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";

type User = { id: string; email: string; channel_org_id: string; status: string; source_code?: string };
type Selection = { user: User; action: "ban" | "unban" | "attribution" };
const actionName = { ban: "封禁", unban: "解封", attribution: "改归因" };
const statusName: Record<string,string> = { active: "正常", banned: "已封禁", disabled: "已停用", pending: "待激活" };
export default function AdminUsersPage() {
 const [filter,setFilter]=useState("");const [reason,setReason]=useState("");const [promo,setPromo]=useState("");
 const [selected,setSelected]=useState<Selection|null>(null);const [operation,setOperation]=useState<(Selection & {reason:string;promo:string})|null>(null);
 const [open,setOpen]=useState(false);const [error,setError]=useState("");const [message,setMessage]=useState("");
 const client=useQueryClient();
 const query=useQuery({queryKey:["/admin/users"],queryFn:async()=>{const r=await fetch(`${apiBase}/admin/users`,{credentials:"include"});const body=await r.json();if(!r.ok)throw new Error(body.error?.message||"读取用户失败，请重试。");return body as {items:User[]};}});
 const items=(query.data?.items||[]).filter(u=>`${u.email} ${u.channel_org_id} ${u.source_code||""}`.toLowerCase().includes(filter.trim().toLowerCase()));
 function select(user:User,action:Selection["action"]){setSelected({user,action});setReason("");setPromo("");setError("");}
 async function submit(){if(!operation)return false;setError("");try{const r=await fetch(`${apiBase}/admin/users/${encodeURIComponent(operation.user.id)}/${operation.action}`,{method:"POST",credentials:"include",headers:confirmHeaders,body:JSON.stringify({reason:operation.reason,...(operation.action==="attribution"?{promotion_code:operation.promo}:{})})});const body=await r.json();if(!r.ok){setError(body.error?.message||"操作未完成，请核对后重试。");return false;}setMessage(`已${actionName[operation.action]} ${operation.user.email}。`);setSelected(null);void client.invalidateQueries({queryKey:["/admin/users"]});return true;}catch{setError("连接中断，尚未确认结果。请关闭确认窗口并刷新用户状态，核对是否已生效后再操作。");return false;}}
 return <AdminShell><section className="rounded-card border border-hairline bg-canvas-raised p-6">
  <AdminH2 k="users" className="mb-4 text-lg font-semibold tracking-tight"/>
  <p className="mb-3 text-sm text-ink-secondary">先选择具体用户和操作，再填写真实原因。封禁会停用登录与旧 API Key，暂停未结算佣金；解封不会恢复已撤销的 API Key。所有变更写入审计，不能封禁自己。</p>
  <div className="mb-3 flex gap-2"><Input aria-label="筛选用户" placeholder="按邮箱、渠道编号或推广码筛选" value={filter} onChange={e=>setFilter(e.target.value)}/><Button variant="outline" disabled={query.isFetching} onClick={()=>void query.refetch()}>刷新用户</Button></div>
  <p className="mb-3 text-sm text-ink-secondary">显示最近 100 位用户，筛选仅作用于当前列表。</p>
  {message&&<p role="status" className="mb-3">{message}</p>}
  {query.isPending&&<p role="status">正在读取用户…</p>}
  {query.isError&&<p role="alert">{query.error.message} 已完成的操作不受列表刷新失败影响。</p>}
  {!query.isPending&&!query.isError&&items.length===0&&<p>{filter?"当前列表没有匹配用户。":"暂无用户。"}</p>}
  <div className="overflow-x-auto"><table className="min-w-full text-left text-sm"><thead><tr className="border-b border-hairline">{["邮箱","渠道编号","推广码","状态","操作"].map(h=><th key={h} className="px-2 py-2">{h}</th>)}</tr></thead><tbody>{items.map(u=><tr key={u.id} className="border-b border-hairline/80"><td className="px-2 py-3 break-all">{u.email}</td><td className="px-2 py-3">{u.channel_org_id||"—"}</td><td className="px-2 py-3">{u.source_code||"—"}</td><td className="px-2 py-3">{statusName[u.status]||u.status}</td><td className="px-2 py-3"><IfCan action="users.write"><div className="flex gap-2"><Button size="sm" variant="outline" disabled={open||query.isError} onClick={()=>select(u,u.status==="banned"?"unban":"ban")}>{u.status==="banned"?"解封":"封禁"}</Button><Button size="sm" variant="outline" disabled={open||query.isError} onClick={()=>select(u,"attribution")}>改归因</Button></div></IfCan></td></tr>)}</tbody></table></div>
  <IfCan action="users.write">{selected&&<section aria-label="填写用户操作原因" className="mt-4 rounded-control border border-hairline p-4"><p>{actionName[selected.action]} · {selected.user.email}</p><label className="mt-3 block">操作原因<Input value={reason} disabled={open} onChange={e=>setReason(e.target.value)} placeholder="请填写此次操作的具体原因"/></label>{selected.action==="attribution"&&<label className="mt-3 block">目标推广码<Input value={promo} disabled={open} onChange={e=>setPromo(e.target.value)} placeholder="请核对新的归属推广码"/></label>}<p className="my-2 text-sm text-ink-secondary">{selected.action==="attribution"?"改归因会影响后续归属与分佣，请先核对目标推广码；不改写历史账单。":"原因必填，确认时会再次展示目标用户与操作后果。"}</p><Button disabled={!reason.trim()||(selected.action==="attribution"&&!promo.trim())||open} onClick={()=>{setOperation({...selected,reason:reason.trim(),promo:promo.trim()});setError("");setOpen(true);}}>核对并继续</Button><Button className="ml-2" variant="outline" disabled={open} onClick={()=>setSelected(null)}>取消填写</Button></section>}</IfCan>
  <ConfirmDialog open={open} onOpenChange={setOpen} title={operation?`确认${actionName[operation.action]}用户`:"确认用户操作"} description={operation?`${operation.user.email}；原因：${operation.reason}。${operation.action==="attribution"?`目标推广码：${operation.promo}，会影响后续归属与分佣。`:operation.action==="ban"?"将停用登录和旧 API Key，暂停未结算佣金。":"恢复登录及正常佣金处理；旧 API Key 不会恢复，请用户重新创建。"}`:""} error={error} onConfirm={submit}/>
 </section></AdminShell>;
}
