"use client";

import { useEffect, useRef, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-button";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { formatOrderDue, formatOrderCredit } from "@/lib/checkout";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";

type Payment = {
  id: string; user_id: string; user_email?: string; user_name?: string; channel_org_id?: string; channel_code?: string;
  adapter: string; purpose: string; status: string; amount_minor: number; credit_minor?: number;
  currency: string; created_at: string; fulfilled_at?: string;
};
const statuses: Record<string,string> = { pending: "待支付", paid: "已支付", refunded: "已退款", failed: "支付失败", expired: "已过期" };
const purposes: Record<string,string> = { wallet: "钱包充值", subscription: "套餐订阅", renewal: "套餐续费" };
const adapters: Record<string,string> = { manual: "线下支付", stripe: "Stripe", alipay: "支付宝", wechat: "微信支付" };
type Selection = { order: Payment; action: "confirm" | "refund" };

export default function AdminPaymentsPage() {
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const [items, setItems] = useState<Payment[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [message, setMessage] = useState("");
  const [selected, setSelected] = useState<Selection | null>(null);
  const [actionError, setActionError] = useState("");
  const [sending, setSending] = useState(false);
  const generation = useRef(0);
  const inFlight = useRef(false);
  const activeFilter = useRef({ search: "", status: "" });

  async function load(filter = activeFilter.current) {
    activeFilter.current = filter;
    const version = ++generation.current;
    setLoading(true); setLoadError("");
    try {
      const params = new URLSearchParams();
      if (filter.search.trim()) params.set("q", filter.search.trim());
      if (filter.status) params.set("status", filter.status);
      const response = await fetch(`${apiBase}/admin/payments?${params}`, { credentials: "include" });
      const body = await response.json();
      if (version !== generation.current) return;
      if (!response.ok || !Array.isArray(body.items)) { setLoadError(body.error?.message || "订单加载失败，请重试。"); return; }
      setItems(body.items);
    } catch { if (version === generation.current) setLoadError("连接失败，请重试加载订单。"); }
    finally { if (version === generation.current) setLoading(false); }
  }
  useEffect(() => { void load(); return () => { ++generation.current; }; }, []);

  async function act() {
    if (!selected || inFlight.current) return false;
    const { order, action } = selected;
    inFlight.current = true; setSending(true); setActionError("");
    try {
      const response = await fetch(`${apiBase}/admin/payments/${encodeURIComponent(order.id)}/${action}`, {
        method: "POST", credentials: "include", headers: confirmHeaders, body: "{}",
      });
      const body = await response.json();
      if (!response.ok) { setActionError(body.error?.message || "操作未完成，请核对订单后重试。"); return false; }
      const result: Payment | undefined = body.item;
      if (!result?.id) throw new Error("missing receipt");
      setMessage(`订单 ${order.id}：${statuses[result.status] || result.status}${result.status === "paid" ? (result.fulfilled_at ? "，额度已到账。" : "，额度尚在入账，请刷新核对。") : "。"}`);
      // Refresh errors must not turn a completed financial action into a failure.
      void load();
      return true;
    } catch { setActionError("暂未确认操作结果。可用原订单重试，或关闭后刷新核对状态。"); return false; }
    finally { inFlight.current = false; setSending(false); }
  }

  function select(order: Payment, action: Selection["action"]) { setActionError(""); setSelected({ order, action }); }
  const order = selected?.order;
  const confirmation = order ? `用户：${order.user_name ? `${order.user_name} · ` : ""}${order.user_email || order.user_id}；渠道：${order.channel_code || order.channel_org_id || "未记录"}；订单：${order.id}；${purposes[order.purpose] || order.purpose}，支付金额 ${formatOrderDue(order)} ${order.currency}${order.purpose === "wallet" ? `，对应 ${formatOrderCredit(order)} USD 充值额度` : ""}。${selected?.action === "confirm" ? "请先核实款项确已收到，确认后会发放对应额度。" : order.adapter === "manual" ? "请先核实线下退款已完成；此操作仅回收平台额度并登记退款，不会转账。" : "将申请支付退款并回收对应额度。若额度无法回收，操作不会完成；消费账单及佣金冲正需在账务流程中另行核对。"}` : "";

  return <AdminShell><section className="space-y-4 rounded-card border border-hairline bg-canvas-raised p-6">
    <AdminH2 k="payments" className="text-lg font-semibold tracking-tight" />
    <p className="text-sm text-ink-secondary">先查找并核对用户、渠道及金额，再从订单操作。商户支付配置由所属渠道管理。</p>
    <form className="flex flex-wrap items-end gap-3" onSubmit={e => { e.preventDefault(); void load({ search, status }); }}>
      <div className="min-w-64 flex-1"><Label htmlFor="payment-search">查找订单</Label><Input id="payment-search" value={search} maxLength={200} onChange={e => setSearch(e.target.value)} placeholder="用户邮箱、姓名、订单号或渠道编号" /></div>
      <div><Label htmlFor="payment-status">订单状态</Label><select id="payment-status" className="block h-10 rounded-control border border-hairline bg-canvas px-3" value={status} onChange={e => setStatus(e.target.value)}><option value="">全部状态</option>{Object.entries(statuses).map(([value,label]) => <option key={value} value={value}>{label}</option>)}</select></div>
      <Button type="submit" disabled={loading || sending}>搜索订单</Button><Button type="button" variant="outline" disabled={loading || sending} onClick={() => void load()}>刷新订单</Button>
    </form>
    {message ? <p role="status" className="text-sm">{message}</p> : null}
    {actionError && !selected ? <p role="alert" className="text-danger">{actionError}</p> : null}
    {loading ? <p role="status">正在加载订单…</p> : loadError ? <div role="alert"><p>{loadError}</p><Button type="button" variant="outline" onClick={() => void load()}>重试</Button></div> : !items.length ? <p>没有符合条件的订单。请核对搜索条件；新支付订单产生后会显示在这里。</p> : <>
      <p className="text-xs text-ink-secondary">显示最新匹配的 {items.length} 笔订单，最多 100 笔。可输入完整邮箱或订单号缩小范围。</p>
      <div className="overflow-x-auto"><table className="w-full min-w-[52rem] table-fixed text-left text-sm"><colgroup>{[20,22,12,18,14,14].map((width,index) => <col key={index} style={{width:`${width}%`}} />)}</colgroup><thead><tr className="border-b border-hairline">{["订单 / 时间","用户 / 渠道","用途 / 支付方式","支付金额 / 充值额度","状态","操作"].map((h,index) => <th className={index === 5 ? "sticky right-0 bg-canvas-raised p-3" : "p-3"} key={h}>{h}</th>)}</tr></thead>
        <tbody>{items.map(item => <tr key={item.id} className="border-b border-hairline">
          <td className="p-3"><span className="break-all font-mono text-xs">{item.id}</span><p className="mt-1 text-ink-secondary">{new Date(item.created_at).toLocaleString()}</p></td>
          <td className="break-all p-3"><p>{item.user_name}</p><p>{item.user_email || item.user_id}</p><p className="text-ink-secondary">{item.channel_code || item.channel_org_id || "渠道未记录"}</p></td>
          <td className="p-3">{purposes[item.purpose] || item.purpose}<p className="text-ink-secondary">{adapters[item.adapter] || item.adapter}</p></td>
          <td className="p-3 tabular-nums">{formatOrderDue(item)} {item.currency}{item.purpose === "wallet" ? <p className="text-ink-secondary">充值额度 {formatOrderCredit(item)} USD</p> : null}</td>
          <td className="p-3">{statuses[item.status] || item.status}{item.status === "paid" ? <p className="text-ink-secondary">{item.fulfilled_at ? "额度已到账" : "额度待入账"}</p> : null}</td>
          <td className="sticky right-0 bg-canvas-raised p-3"><IfCan action="payments.write"><div className="flex flex-wrap gap-2">
            {(item.status === "pending" || (item.status === "paid" && !item.fulfilled_at)) ? <Button type="button" size="sm" disabled={sending} onClick={() => select(item,"confirm")}>{item.status === "pending" ? "确认收款" : "重试入账"}</Button> : null}
            {item.status === "paid" ? <Button type="button" size="sm" variant="outline" disabled={sending} onClick={() => select(item,"refund")}>{item.adapter === "manual" ? "登记退款" : "退款"}</Button> : null}
          </div></IfCan></td>
        </tr>)}</tbody></table></div>
    </>}
    <ConfirmDialog open={!!selected} onOpenChange={open => { if (!open) setSelected(null); }} title={selected?.action === "refund" ? "确认订单退款" : "确认订单收款"} description={confirmation} error={actionError} onConfirm={act} />
  </section></AdminShell>;
}
