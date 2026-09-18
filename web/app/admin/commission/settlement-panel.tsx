"use client";

import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ConfirmDialog } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { IfCan } from "@/components/rbac/if-can";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { formatUsdMinor } from "@/lib/money";

type Settlement = { id: string; amount_minor: number; status: string; period_start: string; period_end: string; channel_code?: string; channel_org_id?: string; beneficiary_role_id?: string; recipient?: { email: string; display_name: string }; payout_reference?: string; reversed_minor?: number };
type Operation = { kind: "unfreeze"; ignoreMinimum: boolean } | { kind: "settle"; ignoreMinimum: boolean } | { kind: "payout"; item: Settlement; reference: string };
const money = (amount: number) => `${formatUsdMinor(amount)} USD`;
const statusName = (status: string) => ({ settled: "待登记打款", paid: "已登记打款", cancelled: "已撤销（佣金变更）" }[status] || status);
const recipient = (item: Settlement) => item.recipient?.email ? `${item.recipient.display_name || "收款人"} · ${item.recipient.email}` : `收款角色 ${item.beneficiary_role_id || "未知"}`;

export function SettlementPanel() {
  const queryClient = useQueryClient();
  const [items, setItems] = useState<Settlement[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [filter, setFilter] = useState("");
  const [ignoreMinimum, setIgnoreMinimum] = useState(false);
  const [selected, setSelected] = useState<Settlement | null>(null);
  const [reference, setReference] = useState("");
  const [operation, setOperation] = useState<Operation | null>(null);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  async function reload() {
    setLoading(true); setLoadError("");
    try {
      const response = await fetch(`${apiBase}/admin/settlements`, { credentials: "include" });
      const body = await response.json();
      if (!response.ok) throw new Error(body.error?.message || "读取结算单失败。");
      setItems(body.items || []);
    } catch (e) { setLoadError(e instanceof Error ? e.message : "读取结算单失败，请重试。"); }
    finally { setLoading(false); }
  }
  useEffect(() => { void reload(); }, []);
  function review(op: Operation) { setOperation(op); setError(""); setOpen(true); }
  async function submit() {
    if (!operation) return false;
    const op = operation;
    const path = op.kind === "unfreeze" ? "/admin/commissions/unfreeze" : op.kind === "settle" ? `/admin/commissions/settle${op.ignoreMinimum ? "?ignore_minimum=1" : ""}` : `/admin/settlements/${encodeURIComponent(op.item.id)}/payout`;
    setError("");
    try {
      const response = await fetch(`${apiBase}${path}`, { method: "POST", credentials: "include", headers: confirmHeaders, body: JSON.stringify(op.kind === "payout" ? { method: "manual", reference: op.reference } : {}) });
      const body = await response.json();
      if (!response.ok) { setError(body.error?.message || "操作未完成，请重试。"); return false; }
      if (op.kind === "payout") {
        setItems(rows => rows.map(row => row.id === op.item.id ? { ...row, status: "paid", payout_reference: op.reference } : row));
        setSelected(null); setReference("");
        setMessage(`已登记 ${recipient(op.item)} 的线下打款 ${money(op.item.amount_minor)}，凭证：${op.reference}。系统没有执行转账。`);
      } else if (op.kind === "unfreeze") setMessage(body.unfrozen ? `已解冻 ${body.unfrozen} 条到期佣金。` : "当前没有已到期且可解冻的佣金，未提前解冻任何记录。");
      else setMessage(body.items?.length ? `已生成 ${body.items.length} 张结算单，请逐单核对收款人和金额。` : "没有符合本次条件的可结算佣金，未生成空结算单。");
      void queryClient.invalidateQueries({ predicate: query => typeof query.queryKey[0] === "string" && query.queryKey[0].startsWith("/admin/commissions") });
      void reload();
      return true;
    } catch { setError("连接中断，尚未确认结果。请按原操作重试；同一结算单及凭证不会重复登记。"); return false; }
  }
  const visible = items.filter(item => `${item.id} ${recipient(item)} ${item.channel_code || item.channel_org_id}`.toLowerCase().includes(filter.trim().toLowerCase()));
  const description = operation?.kind === "payout" ? `${recipient(operation.item)}；渠道 ${operation.item.channel_code || operation.item.channel_org_id || "—"}；结算单 ${operation.item.id}；金额 ${money(operation.item.amount_minor)}；凭证 ${operation.reference}。请确认线下实际已完成打款。系统仅登记，不会转账。`
    : operation?.kind === "unfreeze" ? "只解冻冻结期已结束的佣金，不跳过冻结期，不执行打款。" : `按收款角色汇总当前可结算佣金。${operation?.kind === "settle" && operation.ignoreMinimum ? "本次明确忽略最低结算金额。" : "遵守最低结算金额，未达标佣金留待下次。"}佣金发生冲正后，受影响的未付款结算单会撤销，剩余佣金可重新生成结算单。`;
  return <section aria-label="佣金结算与打款登记" className="rounded-card border border-hairline bg-canvas-raised p-6">
    <h2 className="text-lg font-semibold">佣金结算与打款登记</h2>
    <p className="my-3 text-sm text-ink-secondary">先解冻到期佣金，再生成结算单。线下完成转账后，选择对应结算单登记真实凭证；系统不会自动转账。</p>
    <IfCan action="commission.write"><div className="mb-4 flex flex-wrap items-center gap-3">
      <Button variant="outline" onClick={() => review({ kind: "unfreeze", ignoreMinimum: false })}>解冻到期佣金</Button>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={ignoreMinimum} onChange={e => setIgnoreMinimum(e.target.checked)} />忽略最低结算金额</label>
      <Button onClick={() => review({ kind: "settle", ignoreMinimum })}>生成结算单</Button>
    </div></IfCan>
    {message && <p role="status" className="my-3 text-sm">{message}</p>}
    <div className="flex flex-wrap gap-2"><Input className="max-w-md" aria-label="筛选结算单" placeholder="按收款人邮箱、结算单编号或渠道筛选" value={filter} onChange={e => setFilter(e.target.value)} /><Button variant="outline" onClick={() => void reload()} disabled={loading}>刷新结算单</Button></div>
    <p className="my-2 text-sm text-ink-secondary">显示最近 100 张结算单，筛选仅作用于当前列表。撤销单保留原金额供核对，不可登记打款。</p>
    {loading && <p role="status">正在读取结算单…</p>}
    {loadError && <p role="alert" className="text-danger">{loadError} 已完成的操作不受列表刷新失败影响。</p>}
    {!loading && !loadError && visible.length === 0 && <p className="mt-3 text-sm">{filter ? "当前列表没有匹配的结算单。" : "暂无结算单；到期佣金解冻并达到结算条件后，可生成结算单。"}</p>}
    <ul className="mt-4 grid gap-3">{visible.map(item => <li key={item.id} className="rounded-control border border-hairline p-4 text-sm">
      <div className="flex flex-wrap justify-between gap-2"><p className="break-all font-medium">{recipient(item)}</p><p className="font-mono">{money(item.amount_minor)}</p></div>
      <p className="mt-1">{statusName(item.status)} · 渠道 {item.channel_code || item.channel_org_id || "—"} · 账期 {item.period_start?.slice(0, 7) || "—"}</p>
      <p className="mt-1 break-all text-ink-secondary">结算单：{item.id}</p>
      {item.payout_reference && <p className="mt-1 break-all">打款凭证：{item.payout_reference}</p>}
      {item.status === "cancelled" && <p className="mt-2">该单因佣金冲正已撤销。未冲正的佣金已恢复可结算，请重新生成结算单。</p>}
      {item.status === "paid" && (item.reversed_minor || 0) > 0 && <p className="mt-2 text-danger">打款后佣金冲正 {money(item.reversed_minor!)}，请联系财务核对追回或抵扣安排；原打款记录保留。</p>}
      <IfCan action="commission.write">{item.status === "settled" && <Button className="mt-3" variant="outline" disabled={loading || !!loadError} onClick={() => { setSelected(item); setReference(""); setError(""); }}>登记此单打款</Button>}</IfCan>
    </li>)}</ul>
    <IfCan action="commission.write">{selected && <section aria-label="填写打款凭证" className="mt-4 rounded-control border border-hairline p-4">
      <p className="break-all">{recipient(selected)} · {money(selected.amount_minor)} · {selected.id}</p>
      <label className="mt-3 block text-sm">线下打款凭证<Input value={reference} disabled={open} onChange={e => setReference(e.target.value)} maxLength={200} placeholder="填写真实转账流水号或内部凭证编号" /></label>
      <div className="mt-3 flex gap-2"><Button disabled={!reference.trim() || new TextEncoder().encode(reference.trim()).length > 200 || open} onClick={() => review({ kind: "payout", item: selected, reference: reference.trim() })}>核对并登记</Button><Button variant="outline" disabled={open} onClick={() => setSelected(null)}>取消填写</Button></div>
    </section>}</IfCan>
    <ConfirmDialog open={open} onOpenChange={setOpen} title={operation?.kind === "payout" ? "确认登记线下打款" : operation?.kind === "unfreeze" ? "确认解冻到期佣金" : "确认生成结算单"} description={description} error={error} onConfirm={submit} />
  </section>;
}
