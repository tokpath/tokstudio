"use client";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/confirm-button";
import { LedgerTable } from "@/components/console/ledger-table";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useViewer } from "@/components/rbac/viewer-context";
import { canChannelAction, canWrite } from "@/lib/rbac";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders } from "@/lib/confirm";
import { formatUsdMinor, parseUsdToMinor } from "@/lib/money";
import { beginOperation, finishOperation, loadOperation, type SavedOperation } from "@/lib/stable-operation";

type Supplier = { id: string; channel_org_id: string; amount_minor: number; source_type: string; vendor_name?: string; memo?: string; reversal_of?: string; occurred_at?: string };
type Payload = { amount_minor: number; currency: string; source_type: string; vendor_name: string; memo: string; occurred_at: string };
const localTime = () => new Date(Date.now() - new Date().getTimezoneOffset() * 60000).toISOString().slice(0,16);

export function AdminSupplierPanel({ channelID, prefix = "/admin" }: { channelID?: string; prefix?: "/admin" | "/channel" }) {
  const viewer = useViewer();
  const canEdit = !channelID && (prefix === "/channel" ? canChannelAction("finance", viewer) : canWrite("billing.refund", viewer));
  const storageKey = `supplier:${prefix}:${viewer.userId || 'loading'}:${channelID || 'own-brand'}`;
  const [usd, setUsd] = useState("");
  const [source, setSource] = useState(prefix === "/channel" ? "platform_recharge" : "provider_invoice");
  const [vendor, setVendor] = useState("");
  const [memo, setMemo] = useState("");
  const [when, setWhen] = useState(localTime);
  const [operation, setOperation] = useState<SavedOperation<Payload> | null>(null);
  const [conflict, setConflict] = useState(false);
  const [message, setMessage] = useState("");
  const [reason, setReason] = useState("");
  const path = `${prefix}/supplier-entries${channelID ? `?channel_id=${encodeURIComponent(channelID)}` : ''}`;
  const query = useQuery({ queryKey: [viewer.userId, path], queryFn: () => apiClient<{ items?: Supplier[]; error?: { message?: string } }>("GET", path) });
  useEffect(() => {
    const saved = loadOperation<Payload>(storageKey); setOperation(saved); setConflict(false);
    if (saved) {
      setUsd(String(saved.payload.amount_minor / 1_000_000)); setVendor(saved.payload.vendor_name); setSource(saved.payload.source_type); setMemo(saved.payload.memo);
      const date = new Date(saved.payload.occurred_at); setWhen(new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0,16));
      setMessage("有一笔支出结果待确认，请核对原操作并重试查询。");
    } else { setUsd(""); setVendor(""); setMemo(""); setWhen(localTime()); setMessage(""); }
  }, [storageKey]);
  async function record() {
    const amount = parseUsdToMinor(usd);
    if (!operation && (!amount || !vendor.trim() || !when || !Number.isFinite(new Date(when).getTime()))) { setMessage("请填写付款对象、正数 USD 金额和实际付款时间。"); return false; }
    let pending = operation;
    try {
      pending = beginOperation<Payload>(storageKey, { amount_minor: amount!, currency: "USD", source_type: source, vendor_name: vendor.trim(), memo: memo.trim(), occurred_at: new Date(when).toISOString() });
      setOperation(pending);
      const response = await fetch(`${apiBase}${prefix}/supplier-entries`, { method: "POST", credentials: "include", headers: confirmHeaders, body: JSON.stringify({ ...pending.payload, idempotency_key: pending.id }) });
      const body = await response.json();
      if (!response.ok) {
        if (response.status === 409) setConflict(true);
        if (response.status >= 400 && response.status < 500 && response.status !== 408 && response.status !== 409) { finishOperation(storageKey); setOperation(null); }
        setMessage(body.error?.message || "登记失败，保留原操作与内容。"); return false;
      }
      finishOperation(storageKey); setOperation(null); setUsd(""); setVendor(""); setMemo(""); setWhen(localTime());
      const result = await query.refetch();
      setMessage(`已登记支出 ${body.item?.id}。${result.isError || result.data?.error ? '流水回读失败，请刷新查询；本笔已登记。' : ''}`); return true;
    } catch { setMessage(pending ? `结果待确认，原操作 ${pending.id}。重试只查询/登记同一笔，不能修改金额或对象。` : "无法保存操作身份，尚未提交；请检查浏览器存储。"); return false; }
  }
  async function checkOriginal() {
    if (!operation) return;
    try {
      const response = await fetch(`${apiBase}${prefix}/supplier-entries?operation_id=${encodeURIComponent(operation.id)}`, { credentials: 'include' });
      const body = await response.json();
      if (!response.ok) { setMessage(body.error?.message || '原操作查询失败，请重试。'); return; }
      if (body.operation_status === 'recorded' && body.item) {
        setMessage(`原操作已登记：${body.item.id}，${body.item.vendor_name || '—'}，${formatUsdMinor(body.item.amount_minor)} USD。请核对流水。`);
        finishOperation(storageKey); setOperation(null); setConflict(false); await query.refetch();
      } else if (body.operation_status === 'not_found' && conflict) {
        finishOperation(storageKey); setOperation(null); setConflict(false); setMessage('已确认本账户没有该操作记录。原操作存在身份或参数冲突，可核对后登记新的实际交易。');
      } else { setMessage('尚未查询到已完成记录，可重试同一原操作。'); }
    } catch { setMessage('原操作查询结果未知，请保留当前操作重试。'); }
  }
  async function reverse(item: Supplier) {
    if (!reason.trim()) { setMessage("请填写本次冲正原因。"); return false; }
    try {
      const response = await fetch(`${apiBase}${prefix}/supplier-entries/${encodeURIComponent(item.id)}/reverse`, { method: "POST", credentials: "include", headers: confirmHeaders, body: JSON.stringify({ reason: reason.trim() }) });
      const body = await response.json();
      if (!response.ok) { setMessage(body.error?.message || "冲正失败"); return false; }
      const result = await query.refetch(); setMessage(`已冲正 ${item.id}，原流水保留。${result.isError ? '流水回读失败，请刷新。' : ''}`); return true;
    } catch { setMessage(`冲正结果待确认，请查询原支出 ${item.id}；重试仍使用该原记录。`); return false; }
  }
  return <section className="rounded-card border border-hairline bg-canvas-raised p-6" aria-label="供应商支出">
    <h2 className="mb-3 text-lg font-semibold">供应商支出</h2>
    {canEdit ? <>
      <fieldset disabled={Boolean(operation)} className="mb-3 grid max-w-xl gap-3">
        <label className="grid gap-1 text-sm">付款对象<Input aria-label="付款对象" value={vendor} onChange={e => setVendor(e.target.value)} /></label>
        <label className="grid gap-1 text-sm">实际付款金额（USD）<Input aria-label="实际付款金额（USD）" inputMode="decimal" value={usd} onChange={e => setUsd(e.target.value)} /></label>
        <label className="grid gap-1 text-sm">实际付款时间<Input aria-label="实际付款时间" type="datetime-local" value={when} onChange={e => setWhen(e.target.value)} /></label>
        <label className="grid gap-1 text-sm">付款类别<select className="h-10 rounded-control border border-hairline bg-canvas-raised px-3" aria-label="付款类别" value={source} onChange={e => setSource(e.target.value)}><option value="provider_invoice">付给模型商</option><option value="platform_recharge">付给技术平台</option><option value="other">其他</option></select></label>
        <label className="grid gap-1 text-sm">说明（可选）<Input aria-label="说明（可选）" value={memo} onChange={e => setMemo(e.target.value)} /></label>
      </fieldset>
      {operation ? <p role="status" className="mb-3 text-sm text-hold">待确认：{operation.payload.vendor_name}，{formatUsdMinor(operation.payload.amount_minor)} USD；原操作 {operation.id}。</p> : null}
      <ConfirmButton size="sm" disabled={viewer.loading || !viewer.userId} title="登记已实际付款的支出" description={`付款对象 ${operation?.payload.vendor_name || vendor}，金额 ${formatUsdMinor(operation?.payload.amount_minor ?? parseUsdToMinor(usd))} USD；确认款项已经在线下支付，本次只登记。`} onConfirm={record}>{operation ? "重试原操作" : "登记支出"}</ConfirmButton>
      {operation ? <Button size="sm" variant="outline" className="ml-2" onClick={() => void checkOriginal()}>查询原操作结果</Button> : null}
    </> : null}
    <Button size="sm" variant="outline" className="ml-2" onClick={() => void query.refetch()}>刷新流水</Button>
    {query.isError || query.data?.error ? <p role="alert">流水读取失败，请重试。</p> : null}
    {canEdit ? <label className="my-3 grid max-w-xl gap-1 text-sm">冲正原因<Input aria-label="冲正原因" value={reason} onChange={e => setReason(e.target.value)} /></label> : null}
    <LedgerTable columns={["时间", "对象", "金额（USD）", "说明", "操作"]} emptyDetail="已实际付款的支出会在此保留记录。" emptyTitle={query.isLoading ? "正在读取流水…" : query.isError ? "流水暂不可用" : "暂无供应商支出"} rows={(query.data?.items || []).map(item => ({ key: item.id, cells: [item.occurred_at ? new Date(item.occurred_at).toLocaleString() : '—', item.vendor_name || '—', formatUsdMinor(item.amount_minor), item.memo || '—', item.reversal_of || !canEdit ? '—' : <ConfirmButton key={item.id} size="sm" variant="outline" title="冲正供应商支出" description={`原记录 ${item.id}，${formatUsdMinor(item.amount_minor)} USD；补记反向流水，保留原记录。`} onConfirm={() => reverse(item)}>冲正</ConfirmButton>] }))} />
    {message ? <p role="status" className="mt-3 text-sm">{message}</p> : null}
  </section>;
}
