"use client";

import { useEffect, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { IfCan } from "@/components/rbac/if-can";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { parseUsdToMinor } from "@/lib/money";

type Recovery = { id: string; user_id: string; settlement_id: string; amount_minor: number; recovered_minor: number; status: string; recipient?: { email: string; display_name: string }; receipts: { id: string; amount_minor: number; reference: string; note: string; actor_user_id: string; actor_email?: string; created_at: string }[] };
type Operation = { item: Recovery; amount_minor: number; reference: string; note: string; idempotency_key: string };
const money = (n: number) => `${n / 1_000_000} USD`;
const person = (r: Recovery) => r.recipient?.email ? `${r.recipient.display_name || "收款人"} · ${r.recipient.email}` : `收款用户 ${r.user_id}`;

export function RecoveryPanel({ onRecorded }: { onRecorded: () => void }) {
  const [items, setItems] = useState<Recovery[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [filter, setFilter] = useState("");
  const [selected, setSelected] = useState<Recovery | null>(null);
  const [amount, setAmount] = useState("");
  const [reference, setReference] = useState("");
  const [note, setNote] = useState("");
  const [operation, setOperation] = useState<Operation | null>(null);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  async function reload() {
    setLoading(true); setLoadError("");
    try {
      const response = await fetch(`${apiBase}/admin/commission-recoveries`, { credentials: "include" });
      const body = await response.json();
      if (!response.ok) throw new Error(body.error?.message || "读取佣金追回记录失败。");
      setItems(body.items || []);
    } catch (e) { setLoadError(e instanceof Error ? e.message : "读取失败，请重试。"); }
    finally { setLoading(false); }
  }
  useEffect(() => { void reload(); }, []);
  const remaining = selected ? selected.amount_minor - selected.recovered_minor : 0;
  const minor = parseUsdToMinor(amount);
  const validation = minor === null || minor <= 0 || minor > remaining ? `请输入大于 0 且不超过 ${money(remaining)} 的金额，最多 6 位小数。` : !reference.trim() ? "请填写实际收款凭证。" : new TextEncoder().encode(reference.trim()).length > 200 ? "凭证过长，请缩短至 200 字节以内。" : new TextEncoder().encode(note.trim()).length > 1000 ? "备注过长，请缩短至 1000 字节以内。" : "";
  async function submit() {
    if (!operation) return false;
    setError("");
    const { item, ...payload } = operation;
    try {
      const response = await fetch(`${apiBase}/admin/commission-recoveries/${encodeURIComponent(item.id)}/receipts`, { method: "POST", credentials: "include", headers: confirmHeaders, body: JSON.stringify(payload) });
      const body = await response.json();
      if (!response.ok) { setError(body.error?.message || "登记未完成，请核对后重试。"); return false; }
      setMessage(`已登记 ${person(item)} 收回 ${money(payload.amount_minor)}，凭证：${payload.reference}。以刷新后的剩余金额为准；佣金钱包和消费余额未变动。`);
      setSelected(null); void reload(); onRecorded();
      return true;
    } catch { setError("连接中断，尚未确认结果。请在此窗口按原操作重试，系统不会重复登记。"); return false; }
  }
  const visible = items.filter(r => `${r.id} ${r.settlement_id} ${person(r)}`.toLowerCase().includes(filter.trim().toLowerCase()));
  return <section aria-label="佣金追回与收款登记" className="rounded-card border border-hairline bg-canvas-raised p-6">
    <h2 className="text-lg font-semibold">佣金追回与收款登记</h2>
    <p className="my-3 text-sm text-ink-secondary">仅登记线下已经实际收回的款项，支持分次收回。全部收回后自动结清；此操作不发起转账，不扣钱包余额，也不抵扣后续佣金。</p>
    {message && <p role="status" className="my-3">{message}</p>}
    <div className="flex gap-2"><Input aria-label="筛选追回记录" placeholder="按收款人邮箱、结算单或追回编号筛选" value={filter} onChange={e => setFilter(e.target.value)} /><Button variant="outline" disabled={loading} onClick={() => void reload()}>刷新追回记录</Button></div>
    <p className="my-2 text-sm text-ink-secondary">最多显示 100 条记录，待收回优先。筛选仅作用于当前列表。</p>
    {loading && <p role="status">正在读取追回记录…</p>}
    {loadError && <p role="alert">{loadError} 已成功登记的款项不受刷新失败影响。</p>}
    {!loading && !loadError && visible.length === 0 && <p>{filter ? "当前列表没有匹配记录。" : "暂无需要追回的佣金记录。"}</p>}
    <ul className="grid gap-3">{visible.map(r => <li key={r.id} className="rounded-control border border-hairline p-4 text-sm">
      <p className="break-all font-medium">{person(r)}</p>
      <p className="mt-2">{r.status === "closed" ? "已结清" : "待收回"} · 原需收回 {money(r.amount_minor)} · 已收回 {money(r.recovered_minor)} · 剩余 {money(r.amount_minor - r.recovered_minor)}</p>
      <p className="mt-1 break-all text-ink-secondary">追回编号：{r.id} · 结算单：{r.settlement_id}</p>
      <ul className="mt-2 space-y-2">{r.receipts.map(receipt => <li key={receipt.id} className="break-all">已收回 {money(receipt.amount_minor)} · 凭证：{receipt.reference} · {new Date(receipt.created_at).toLocaleString()}<p className="text-ink-secondary">登记人：{receipt.actor_email || receipt.actor_user_id}{receipt.note && ` · 内部备注：${receipt.note}`}</p></li>)}</ul>
      <IfCan action="commission.write">{r.status === "pending" && <Button className="mt-3" variant="outline" disabled={loading || !!loadError || open} onClick={() => { setSelected(r); setAmount(String((r.amount_minor - r.recovered_minor) / 1_000_000)); setReference(""); setNote(""); }}>登记已收回款项</Button>}</IfCan>
    </li>)}</ul>
    <IfCan action="commission.write">{selected && <section aria-label="填写收回凭证" className="mt-4 rounded-control border border-hairline p-4">
      <p>{person(selected)} · 待收回 {money(remaining)}</p>
      <label className="mt-3 block">本次实际收回金额（USD）<Input value={amount} disabled={open} inputMode="decimal" onChange={e => setAmount(e.target.value)} /></label>
      <label className="mt-3 block">实际收款凭证<Input value={reference} disabled={open} onChange={e => setReference(e.target.value)} /></label>
      <label className="mt-3 block">内部备注（可选）<Input value={note} disabled={open} onChange={e => setNote(e.target.value)} /></label>
      {validation && <p className="mt-2 text-sm">{validation}</p>}
      <div className="mt-3 flex gap-2"><Button disabled={!!validation || open} onClick={() => { setOperation({ item: selected, amount_minor: minor!, reference: reference.trim(), note: note.trim(), idempotency_key: crypto.randomUUID() }); setError(""); setOpen(true); }}>核对收回款项</Button><Button variant="outline" disabled={open} onClick={() => setSelected(null)}>取消填写</Button></div>
    </section>}</IfCan>
    <ConfirmDialog open={open} onOpenChange={setOpen} title="确认登记实际收回款项" description={operation ? `${person(operation.item)}；结算单 ${operation.item.settlement_id}；本次实际收回 ${money(operation.amount_minor)}；凭证 ${operation.reference}；内部备注 ${operation.note || "无"}；登记后预计剩余 ${money(operation.item.amount_minor - operation.item.recovered_minor - operation.amount_minor)}。请核对真实到账凭证，未到账不要登记。此操作只记录收款，不改变任何钱包余额。` : ""} error={error} onConfirm={submit} />
  </section>;
}
