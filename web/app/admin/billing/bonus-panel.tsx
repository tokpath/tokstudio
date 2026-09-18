"use client";

import { useEffect, useRef, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { formatUsdMinor, parseUsdToMinor } from "@/lib/money";
import { useViewer } from "@/components/rbac/viewer-context";

type Recipient = { id: string; email: string; display_name: string; channel_code: string; status: string };
type Operation = { key: string; recipient: Recipient; amount: number };

export function BonusPanel() {
  const viewer = useViewer();
  const storageKey = `bonus-pending:${viewer.userId}`;
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Recipient[]>([]);
  const [selected, setSelected] = useState<Recipient | null>(null);
  const [amount, setAmount] = useState("1");
  const [searching, setSearching] = useState(false);
  const [searched, setSearched] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [operation, setOperation] = useState<Operation | null>(null);
  const [open, setOpen] = useState(false);
  const [sending, setSending] = useState(false);
  const searchVersion = useRef(0);
  const inFlight = useRef(false);
  const minor = parseUsdToMinor(amount);

  useEffect(() => {
    try {
      const saved = sessionStorage.getItem(storageKey);
      if (saved) {
        const pending = JSON.parse(saved) as Operation;
        if (pending.key && pending.recipient?.id && pending.amount > 0) {
          setOperation(pending);
          setMessage("有一笔赠送尚未确认结果，请先核对原操作。重试不会重复发放。");
        }
      }
    } catch { /* A corrupt draft must not submit automatically. */ }
  }, [storageKey]);

  async function search() {
    const version = ++searchVersion.current;
    setSearching(true); setError(""); setSelected(null); setResults([]); setSearched(false);
    try {
      const response = await fetch(`${apiBase}/admin/billing/users?q=${encodeURIComponent(query.trim())}`, { credentials: "include" });
      const body = await response.json();
      if (version !== searchVersion.current) return;
      if (!response.ok) throw new Error(body.error?.message || "搜索失败，请重试。");
      setResults(body.items || []); setSearched(true);
    } catch (e) {
      if (version === searchVersion.current) setError(e instanceof Error ? e.message : "搜索失败，请重试。");
    } finally { if (version === searchVersion.current) setSearching(false); }
  }

  function review() {
    setError("");
    if (!operation && selected && minor !== null && minor > 0) {
      setOperation({ key: crypto.randomUUID(), recipient: selected, amount: minor });
    }
    setOpen(true);
  }

  async function grant() {
    if (!operation || inFlight.current) return false;
    // Save before sending, so a lost response or page reload retries the same operation.
    try { sessionStorage.setItem(storageKey, JSON.stringify(operation)); }
    catch { setError("浏览器无法保存操作记录，请允许此站点存储后重试。"); return false; }
    inFlight.current = true; setSending(true); setError("");
    try {
      const response = await fetch(`${apiBase}/admin/entitlements/bonus`, {
        method: "POST", credentials: "include",
        headers: { ...confirmHeaders, "Idempotency-Key": operation.key },
        body: JSON.stringify({ user_id: operation.recipient.id, unit_type: "usd_credit", amount: operation.amount, expires_in_seconds: 86400 }),
      });
      const body = await response.json();
      if (!response.ok) {
        setError(body.error?.message || "发放未完成，请重试核对结果。");
        // Validation/auth failures are definitive. Conflicts and server failures retain the operation.
        if ([400, 401, 403].includes(response.status)) { sessionStorage.removeItem(storageKey); }
        return false;
      }
      sessionStorage.removeItem(storageKey);
      setMessage(`已向 ${operation.recipient.email} 赠送 ${formatUsdMinor(operation.amount)} USD。用户可在「套餐」查看剩余额度与到期时间。`);
      setOperation(null); setSelected(null); setResults([]); setSearched(false); setQuery("");
      return true;
    } catch {
      setError("暂未确认发放结果。请重试核对原操作，不会重复发放。");
      return false;
    } finally { inFlight.current = false; setSending(false); }
  }

  function close(next: boolean) {
    setOpen(next);
    if (!next && !inFlight.current) {
      try { if (!sessionStorage.getItem(storageKey)) setOperation(null); } catch { /* Keep the draft. */ }
    }
  }

  return <section className="my-5 space-y-3 rounded-control border border-hairline p-4" aria-label="赠送额度">
    <h3 className="font-semibold">赠送额度</h3>
    <p className="text-sm text-ink-secondary">发放后 24 小时到期，优先抵扣 API 消费；与钱包余额分开记录，不可提现。</p>
    <form className="flex flex-wrap items-end gap-2" onSubmit={e => { e.preventDefault(); if (query.trim().length >= 2 && !operation) void search(); }}>
      <div className="min-w-64 flex-1"><Label htmlFor="bonus-user-search">收款用户</Label>
        <Input id="bonus-user-search" placeholder="输入邮箱、姓名或用户编号" value={query} maxLength={200} disabled={!!operation}
          onChange={e => { ++searchVersion.current; setQuery(e.target.value); setSelected(null); setResults([]); setSearched(false); setSearching(false); setError(""); }} />
      </div>
      <Button type="submit" variant="outline" disabled={query.trim().length < 2 || searching || !!operation}>{searching ? "搜索中…" : "搜索用户"}</Button>
    </form>
    {searched && !results.length ? <p role="status">未找到用户，请核对邮箱或尝试更完整的姓名。</p> : null}
    {results.length ? <div><p className="text-sm text-ink-secondary">请选择收款用户；最多显示 20 人，可输入完整邮箱缩小范围。</p>
      <ul className="mt-2 space-y-2">{results.map(user => <li key={user.id}>
        <Button className="h-auto w-full justify-start whitespace-normal text-left" variant={selected?.id === user.id ? "default" : "outline"}
          disabled={user.status !== "active" || !!operation} type="button" onClick={() => { setSelected(user); setMessage(""); }} aria-pressed={selected?.id === user.id}>
          {user.display_name || "未设置姓名"} · {user.email} · 渠道 {user.channel_code || "未分配"}{user.status !== "active" ? "（已停用）" : ""}
        </Button>
      </li>)}</ul></div> : null}
    <div><Label htmlFor="bonus-usd">赠送金额（USD）</Label><Input id="bonus-usd" className="w-48" inputMode="decimal" value={amount} disabled={!!operation} onChange={e => setAmount(e.target.value)} aria-describedby="bonus-help" />
      <p id="bonus-help" className="mt-1 text-xs text-ink-secondary">大于 0，最多 6 位小数；24 小时后到期。</p></div>
    <Button type="button" size="sm" onClick={review} disabled={sending || (!operation && (!selected || minor === null || minor <= 0))}>{operation ? "核对原操作" : "赠送额度"}</Button>
    {error && !open ? <p role="alert" className="text-sm text-danger">{error}</p> : null}
    {message ? <p role="status" className="text-sm">{message}</p> : null}
    <ConfirmDialog open={open} onOpenChange={close} title="确认赠送额度" error={error} onConfirm={grant}
      description={operation ? `向 ${operation.recipient.display_name || "用户"}（${operation.recipient.email}，渠道 ${operation.recipient.channel_code || "未分配"}）赠送 ${formatUsdMinor(operation.amount)} USD，有效期 24 小时。操作会写入审计。` : ""} />
  </section>;
}
