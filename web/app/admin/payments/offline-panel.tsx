"use client";
import { useRef, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { parseUsdToMinor, formatUsdMinor } from "@/lib/money";

type Customer = { id: string; email: string; display_name: string; channel_code: string };
type Receipt = { user_id: string; amount_minor: number; credit_minor: number; currency: string; reference: string };
export function OfflineReceiptPanel({ basePath, onRecorded }: { basePath: string; onRecorded: () => void }) {
  const [expanded, setExpanded] = useState(false);
  const [search, setSearch] = useState("");
  const [customers, setCustomers] = useState<Customer[]>([]);
  const [customer, setCustomer] = useState<Customer | null>(null);
  const [amount, setAmount] = useState("");
  const [credit, setCredit] = useState("");
  const [currency, setCurrency] = useState("CNY");
  const [reference, setReference] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [searching, setSearching] = useState(false);
  const [receipt, setReceipt] = useState<Receipt | null>(null);
  const searchingVersion = useRef(0);
  const submitting = useRef(false);
  async function findCustomers() {
    const version = ++searchingVersion.current;
    setError(""); setCustomer(null); setCustomers([]); setSearching(true);
    try {
      const response = await fetch(`${apiBase}${basePath}/recipients?q=${encodeURIComponent(search.trim())}`, { credentials: "include" });
      const body = await response.json();
      if (version !== searchingVersion.current) return;
      if (!response.ok || !Array.isArray(body.items)) throw new Error(body.error?.message || "搜索失败");
      setCustomers(body.items);
      if (!body.items.length) setMessage("没有匹配的本品牌客户。"); else setMessage("");
    } catch (e) { if (version === searchingVersion.current) setError(e instanceof Error ? e.message : "客户搜索失败"); }
    finally { if (version === searchingVersion.current) setSearching(false); }
  }
  function review() {
    setError("");
    const creditMinor = parseUsdToMinor(credit);
    const payMinor = currency === "USD" ? parseUsdToMinor(amount) : /^\d+(\.\d{1,2})?$/.test(amount.trim()) ? Math.round(Number(amount) * 100) : null;
    if (!customer || !creditMinor || creditMinor <= 0 || !payMinor || payMinor <= 0 || !Number.isSafeInteger(payMinor) || !reference.trim() || new TextEncoder().encode(reference.trim()).length > 200) {
      setError("请选择客户，填写有效的实收金额、发放额度和收款凭证。"); return;
    }
    setReceipt({ user_id: customer.id, amount_minor: payMinor, credit_minor: creditMinor, currency, reference: reference.trim() });
  }
  async function record() {
    if (!receipt || submitting.current) return false;
    submitting.current = true; setError("");
    try {
      const response = await fetch(`${apiBase}${basePath}/offline`, { method: "POST", credentials: "include", headers: confirmHeaders, body: JSON.stringify(receipt) });
      const body = await response.json();
      if (!response.ok) { setError(body.error?.message || "划拨失败"); return false; }
      if (!body.item?.fulfilled_at) throw new Error("未确认入账结果，请用原凭证重试。");
      setMessage(`已为 ${customer?.email} 划拨 ${formatUsdMinor(receipt.credit_minor)} USD，长期有效。凭证：${receipt.reference}`);
      setExpanded(false); setAmount(""); setCredit(""); setReference("");
      onRecorded(); return true;
    } catch (e) { setError(e instanceof Error ? e.message : "连接中断，请用原凭证重试。"); return false; }
    finally { submitting.current = false; }
  }
  return <div className="space-y-3">
    <Button variant="outline" onClick={() => { setExpanded(v => !v); setError(""); }}>{expanded ? "收起划拨" : "线下收款划拨"}</Button>
    {expanded && <section aria-label="线下收款划拨" className="space-y-4 rounded-control border border-hairline p-4">
      <p className="text-sm text-ink-secondary">核实款项已进入本品牌账户后，给客户发放长期有效额度。</p>
      <form className="flex gap-2" onSubmit={e => { e.preventDefault(); void findCustomers(); }}>
        <Input aria-label="查找划拨客户" placeholder="客户邮箱、姓名或编号，至少 2 个字符" value={search} onChange={e => { ++searchingVersion.current; setSearch(e.target.value); setCustomer(null); setCustomers([]); setSearching(false); }} maxLength={200} />
        <Button type="submit" disabled={search.trim().length < 2 || searching}>查找客户</Button>
      </form>
      <ul className="space-y-2">{customers.map(item => <li key={item.id}><button type="button" aria-pressed={customer?.id === item.id} className={`w-full rounded-control border p-3 text-left text-sm ${customer?.id === item.id ? "border-brand-emphasis bg-brand-soft" : "border-hairline"}`} onClick={() => setCustomer(item)}>{item.display_name} · {item.email}<span className="ml-2 text-ink-secondary">{item.channel_code || "平台直属"}</span></button></li>)}</ul>
      <div className="grid gap-3 md:grid-cols-4">
        <div><Label htmlFor="offline-currency">实收币种</Label><select id="offline-currency" className="h-10 w-full rounded-control border border-hairline bg-canvas px-3" value={currency} onChange={e => setCurrency(e.target.value)}><option>CNY</option><option>USD</option></select></div>
        <div><Label htmlFor="offline-amount">实收金额</Label><Input id="offline-amount" inputMode="decimal" value={amount} onChange={e => setAmount(e.target.value)} placeholder="100.00" /></div>
        <div><Label htmlFor="offline-credit">发放额度（USD）</Label><Input id="offline-credit" inputMode="decimal" value={credit} onChange={e => setCredit(e.target.value)} placeholder="10.00" /></div>
        <div><Label htmlFor="offline-reference">收款凭证</Label><Input id="offline-reference" maxLength={200} value={reference} onChange={e => setReference(e.target.value)} placeholder="银行流水号或内部凭证编号" /></div>
      </div>
      <Button disabled={!customer} onClick={review}>核对并划拨</Button>
    </section>}
    {message && <p role="status" className="text-sm">{message}</p>}
    {error && !receipt && <p role="alert" className="text-danger">{error}</p>}
    <ConfirmDialog open={!!receipt} onOpenChange={open => { if (!open) setReceipt(null); }} title="确认线下收款划拨" description={receipt ? `${customer?.display_name} · ${customer?.email}；实收 ${receipt.amount_minor / (receipt.currency === "CNY" ? 100 : 1_000_000)} ${receipt.currency}；划拨 ${formatUsdMinor(receipt.credit_minor)} USD，长期有效；凭证 ${receipt.reference}。请确认款项已经收到。` : ""} error={error} onConfirm={record} />
  </div>;
}
