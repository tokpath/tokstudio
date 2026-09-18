"use client";

import { useRef, useState } from "react";
import Link from "next/link";
import { ConfirmDialog } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { formatUsdMinor } from "@/lib/money";

type Preview = { request_id: string; user_id: string; model_id: string; state: string; amount_minor: number; wallet_minor: number; entitlement_minor: number; gift_minor: number };
type Review = { item: Preview; user: { email: string; display_name: string } };
const money = (value: number) => `${formatUsdMinor(value)} USD`;

export function ChargeRefundPanel({ onRefund }: { onRefund: () => void }) {
  const [requestID, setRequestID] = useState("");
  const [review, setReview] = useState<Review | null>(null);
  const [searching, setSearching] = useState(false);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const version = useRef(0);

  async function lookup() {
    const current = ++version.current;
    setSearching(true); setReview(null); setError(""); setMessage("");
    try {
      const response = await fetch(`${apiBase}/admin/refunds/preview?request_id=${encodeURIComponent(requestID.trim())}`, { credentials: "include" });
      const body = await response.json();
      if (current !== version.current) return;
      if (!response.ok) throw new Error(body.error?.message || "读取账单失败，请重试。");
      setReview(body);
    } catch (e) {
      if (current === version.current) setError(e instanceof Error ? e.message : "读取账单失败，请重试。");
    } finally { if (current === version.current) setSearching(false); }
  }

  async function refund() {
    if (!review) return false;
    setError("");
    try {
      const response = await fetch(`${apiBase}/admin/refunds`, { method: "POST", credentials: "include", headers: confirmHeaders, body: JSON.stringify({ request_id: review.item.request_id }) });
      const body = await response.json();
      if (!response.ok) { setError(body.error?.message || "退款未完成，请重试。"); return false; }
      setReview({ ...review, item: { ...review.item, state: "reversed" } });
      setMessage(`已完成 ${review.item.request_id} 的消费退款，相关佣金已同步冲正。充值积分退回 ${money(review.item.wallet_minor)}；恢复原套餐/限时额度 ${money(review.item.entitlement_minor)}，原有效期不变。`);
      onRefund();
      return true;
    } catch {
      setError("连接中断，尚未确认退款结果。请重试同一笔退款，不会重复退回。");
      return false;
    }
  }

  const item = review?.item;
  const person = review ? `${review.user.display_name || "用户"} · ${review.user.email || review.item.user_id}` : "";
  return <section aria-label="消费退款" className="mb-5 rounded-control border border-hairline p-4">
    <h3 className="font-semibold">消费退款</h3>
    <p className="my-2 text-sm text-ink-secondary">将已扣的充值积分退回用户钱包，并恢复原套餐/限时额度、冲正相关佣金。钱包赠送积分不退回；此操作不向银行卡或支付账户转账。</p>
    <form className="flex flex-wrap items-end gap-2" onSubmit={e => { e.preventDefault(); if (requestID.trim() && !searching) void lookup(); }}>
      <label className="min-w-0 flex-1 text-sm">消费请求编号
        <Input className="mt-1" value={requestID} disabled={open} placeholder="从用量/账单页复制请求编号" onChange={e => { version.current++; setSearching(false); setRequestID(e.target.value); setReview(null); setError(""); setMessage(""); }} />
      </label>
      <Button type="submit" variant="outline" disabled={!requestID.trim() || searching || open}>{searching ? "正在查询…" : "查询退款账单"}</Button>
      <Link className="py-2 text-sm underline" href="/admin/usage">查看用量/账单</Link>
    </form>
    {review && item && <div className="mt-3 space-y-2 break-words text-sm">
      <p>退款用户：{person}</p>
      <p>模型：{item.model_id || "—"} · 原消费：{money(item.amount_minor)}</p>
      <p>退回充值积分：{money(item.wallet_minor)} · 恢复原套餐/限时额度：{money(item.entitlement_minor)}</p>
      <p>不退回的钱包赠送积分：{money(item.gift_minor)}。恢复额度保留原有效期，已过期额度不能继续使用。</p>
      {item.state === "reversed" ? <p role="status">该账单已退款，无需再次操作。</p> : item.state === "committed" ? <Button variant="outline" onClick={() => { setError(""); setOpen(true); }}>核对并退消费账单</Button> : <p role="status">该账单当前不能退款。</p>}
    </div>}
    {error && !open && <p role="alert" className="mt-2 text-sm text-danger">{error}</p>}
    {message && <p role="status" className="mt-2 text-sm">{message}</p>}
    <ConfirmDialog open={open} onOpenChange={setOpen} title="确认退消费账单" confirmLabel="确认退款" error={error}
      description={item ? `${person}，请求编号 ${item.request_id}。退回充值积分 ${money(item.wallet_minor)}，恢复原套餐/限时额度 ${money(item.entitlement_minor)}（原有效期不变）；钱包赠送积分 ${money(item.gift_minor)} 不退回。相关佣金同步冲正。` : ""} onConfirm={refund} />
  </section>;
}
