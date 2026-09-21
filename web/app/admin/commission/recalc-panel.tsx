"use client";
import { useState } from "react";
import { ConfirmDialog } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
export function RecalcPanel() {
 const [reference,setReference]=useState("");const [operation,setOperation]=useState("");const [open,setOpen]=useState(false);const [error,setError]=useState("");const [message,setMessage]=useState("");
 async function submit(){setError("");try{const r=await fetch(`${apiBase}/admin/commissions/recalc`,{method:"POST",credentials:"include",headers:confirmHeaders,body:JSON.stringify({usage_event_id:operation})});const body=await r.json();if(!r.ok){setError(body.error?.message||"核对未完成，请重试。");return false;}setMessage(body.item?.changed?"已按原计算快照修正冻结佣金，保留冲正记录和原冻结到期时间。":body.item?.status==="reversed"?"该笔消费已冲正，不会重新生成佣金。":body.item?.status==="no_active_commission"?"该笔消费没有待核对的有效佣金，未新增或恢复佣金。":"核对完成，原佣金与计算快照一致，金额和冻结到期时间均未改变。");return true;}catch{setError("连接中断，尚未确认结果。请按原编号重试，已正确的佣金不会重复修改。");return false;}}
 return <section className="rounded-card border border-hairline bg-canvas-raised p-6" aria-label="佣金核对与重算"><h2 className="text-lg font-semibold">佣金核对与重算</h2><p className="my-3 text-sm text-ink-secondary">使用原消费价格、分佣策略和收款人快照核对。正确记录保持不变，仅修正仍在冻结期且有完整快照的差额；历史缺少快照或已进入结算的差额需财务人工核对。</p><label className="block text-sm">消费请求编号或用量编号<Input value={reference} disabled={open} onChange={e=>setReference(e.target.value)} placeholder="从用量/账单页面复制对应编号" /></label><Button className="mt-3" disabled={!reference.trim()||open} onClick={()=>{setOperation(reference.trim());setError("");setOpen(true);}}>核对佣金</Button>{message&&<p role="status" className="mt-3 text-sm">{message}</p>}<ConfirmDialog open={open} onOpenChange={setOpen} title="确认核对佣金" description={`核对消费 ${operation}。不使用当前新策略覆盖历史，不恢复已退款佣金，不重复修改正确记录。修正失败会整体回滚。`} error={error} onConfirm={submit}/></section>;
}
