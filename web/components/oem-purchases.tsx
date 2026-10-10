"use client";

import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { useViewer } from "@/components/rbac/viewer-context";
import { canWrite } from "@/lib/rbac";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { formatUsdMinor, parseUsdToMinor } from "@/lib/money";
import { formatPayMinor } from "@/lib/payment-quote";
import { beginOperation, finishOperation, loadOperation, type SavedOperation } from "@/lib/stable-operation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { OEMPurchaseReversal } from "./oem-purchase-reversal";
import { ConfirmButton } from "@/components/confirm-button";

type Purchase = {
  id: string; operation_id: string; oem_channel_org_id: string;
  cash_amount_minor: number; cash_currency: string; sale_amount_minor: number; quota_amount_minor: number;
  occurred_at: string; completed_at: string; status?: string; reversed_at?: string; reversal_reason?: string; reversal_operation_id?: string; external_reference?: string; note?: string; actor_user_id?: string;
};
type Payload = Omit<Purchase, "id" | "operation_id" | "completed_at" | "actor_user_id"> & { confirmed: true };
function localTime() {
  return new Date(Date.now() - new Date().getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}
function matchesOperation(p: Purchase, op: SavedOperation<Payload>) {
  const original = op.payload;
  return p.operation_id === op.id && p.oem_channel_org_id === original.oem_channel_org_id &&
    p.cash_currency === original.cash_currency && p.cash_amount_minor === original.cash_amount_minor &&
    p.sale_amount_minor === original.sale_amount_minor && p.quota_amount_minor === original.quota_amount_minor &&
    new Date(p.occurred_at).getTime() === new Date(original.occurred_at).getTime() &&
    (p.external_reference || "") === (original.external_reference || "") && (p.note || "") === (original.note || "");
}

export function OEMPurchasesPanel({ ownerID, channel = false, allowCreate = false }: { ownerID?: string; channel?: boolean; allowCreate?: boolean }) {
  const t = useTranslations("oemPurchase"), viewer = useViewer(), client = useQueryClient();
  const writable = !channel && allowCreate && Boolean(ownerID) && canWrite("channels.quota", viewer);
  const key = `oem-purchase:${viewer.userId}:${ownerID || "platform"}`;
  const activeKey = useRef(key); activeKey.current = key;
  const [operation, setOperation] = useState<SavedOperation<Payload> | null>(null);
  const [cash, setCash] = useState(""), [currency, setCurrency] = useState("USD"), [sale, setSale] = useState(""), [quota, setQuota] = useState("");
  const [when, setWhen] = useState(localTime), [reference, setReference] = useState(""), [note, setNote] = useState("");
  const [confirmed, setConfirmed] = useState(false), [message, setMessage] = useState(""), [cursor, setCursor] = useState("");
  const [rejected, setRejected] = useState(false), [originalPurchase, setOriginalPurchase] = useState<Purchase | null>(null);
  function clearForm() {
    setCash(""); setSale(""); setQuota(""); setReference(""); setNote(""); setConfirmed(false); setWhen(localTime());
  }
  useEffect(() => {
    setOperation(loadOperation<Payload>(key)); clearForm(); setCurrency("USD"); setMessage(""); setCursor(""); setRejected(false); setOriginalPurchase(null);
  }, [key]);
  const path = `${channel ? "/channel" : "/admin"}/oem-purchases?limit=30${ownerID && !channel ? `&oem_channel_id=${encodeURIComponent(ownerID)}` : ""}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`;
  const query = useQuery({ queryKey: [viewer.userId, ownerID, path], enabled: viewer.signedIn, queryFn: async () => {
    const r = await fetch(`${apiBase}${path}`, { credentials: "include" }); const b = await r.json();
    if (!r.ok || !Array.isArray(b.items)) throw new Error(b.error?.message || t("readFailed"));
    return b as { items: Purchase[]; next_cursor?: string };
  } });
  const cashMinor = currency === "USD" ? parseUsdToMinor(cash) : /^\d+(\.\d{1,2})?$/.test(cash.trim()) ? Math.round(Number(cash) * 100) : null;
  const payload: Payload = operation?.payload || {
    oem_channel_org_id: ownerID || "", cash_amount_minor: cashMinor || 0, cash_currency: currency,
    sale_amount_minor: currency === "USD" ? cashMinor || 0 : parseUsdToMinor(sale) || 0, quota_amount_minor: parseUsdToMinor(quota) || 0,
    occurred_at: when && Number.isFinite(new Date(when).getTime()) ? new Date(when).toISOString() : "", external_reference: reference.trim(), note: note.trim(), confirmed: true,
  };
  function validate() {
    if (operation) return !rejected;
    const valid = Boolean(confirmed && [payload.cash_amount_minor, payload.sale_amount_minor, payload.quota_amount_minor].every(n => n > 0 && Number.isSafeInteger(n)) && payload.occurred_at && new Date(payload.occurred_at).getTime() <= Date.now() + 300000);
    setMessage(valid ? "" : t("invalid")); return valid;
  }
  async function completed(p: Purchase, pending: SavedOperation<Payload>) {
    if (activeKey.current !== key) return false;
    if (!matchesOperation(p, pending)) { setMessage(t("mismatch")); return false; }
    finishOperation(key); setOperation(null); clearForm(); setRejected(false); setOriginalPurchase(null); setMessage(t(p.status === "reversed" ? "reversedOriginal" : "completed", { id: p.id }));
    await Promise.allSettled([query.refetch(), client.invalidateQueries({ predicate: q => q.queryKey.some(k => typeof k === "string" && (k.includes("billing-report") || k.includes("dashboard") || k.includes("channel-quotas") || k.includes("pnl") || k.includes("oem-deliveries"))) })]);
    return true;
  }
  async function submit() {
    if (!validate()) return false;
    try {
      const pending = beginOperation(key, payload); setOperation(pending);
      const r = await fetch(`${apiBase}/admin/oem-purchases`, { method: "POST", credentials: "include", headers: confirmHeaders, body: JSON.stringify({ ...pending.payload, operation_id: pending.id }) }); const b = await r.json();
      if (activeKey.current !== key) return false;
      if (!r.ok || !b.item?.id) {
        if (r.status === 409 && ["external_reference_conflict", "operation_conflict"].includes(b.error?.code)) { setRejected(true); setOriginalPurchase(b.error.param?.item || null); }
        else if (r.status >= 400 && r.status < 500 && ![408, 409, 429].includes(r.status)) { finishOperation(key); setOperation(null); }
        setMessage(b.error?.message || t("unknown")); return false;
      }
      return await completed(b.item, pending);
    } catch { if (activeKey.current === key) setMessage(t("unknown")); return false; }
  }
  async function lookup() {
    if (!operation) return;
    try {
      const r = await fetch(`${apiBase}/admin/oem-purchases/operations?oem_channel_id=${encodeURIComponent(ownerID!)}&operation_id=${encodeURIComponent(operation.id)}`, { credentials: "include" }); const b = await r.json();
      if (activeKey.current !== key) return;
      if (r.ok && b.item?.id) {
        if (!matchesOperation(b.item, operation)) { setRejected(true); setOriginalPurchase(b.item); setMessage(t("mismatch")); }
        else await completed(b.item, operation);
      } else setMessage(b.error?.message || t("unknown"));
    } catch { if (activeKey.current === key) setMessage(t("unknown")); }
  }
  function endRejected() { finishOperation(key); setOperation(null); setRejected(false); setOriginalPurchase(null); setMessage(t("ended")); return true; }
  function summary(p: Payload | Purchase) {
    return t("summary", { owner: p.oem_channel_org_id, cash: `${formatPayMinor(p.cash_currency, p.cash_amount_minor)} ${p.cash_currency}`, sale: formatUsdMinor(p.sale_amount_minor), quota: formatUsdMinor(p.quota_amount_minor), time: p.occurred_at ? new Date(p.occurred_at).toLocaleString(undefined, { timeZone: "Asia/Shanghai" }) : "—" });
  }
  return <section id="procurement" className="space-y-4 rounded-card border border-hairline bg-canvas-raised p-5">
    <h2 className="text-lg font-semibold">{t(channel ? "buyerTitle" : "title")}</h2><p className="text-sm text-ink-secondary">{t(channel ? "buyerHint" : "hint")}</p>
    {message && <p role="status">{message}</p>}
    {writable && <div className="space-y-3">
      {operation ? <p role="alert" className="whitespace-pre-line rounded-control border border-hairline p-3">{t(rejected ? "rejected" : "unknown")}{"\n"}{summary(operation.payload)}{"\n"}{t("original", { id: operation.id })}</p> : <div className="grid gap-3 sm:grid-cols-2">
        <label>{t("currency")}<select value={currency} onChange={e => { setCurrency(e.target.value); setCash(""); setSale(""); }} className="block h-10 w-full border border-hairline bg-canvas"><option>USD</option><option>CNY</option></select></label>
        <label>{t("cash")}<Input value={cash} onChange={e => setCash(e.target.value)} inputMode="decimal" /></label>
        {currency === "CNY" && <label>{t("sale")}<Input aria-label={t("sale")} value={sale} onChange={e => setSale(e.target.value)} inputMode="decimal" /><span className="text-xs text-ink-mute">{t("saleHint")}</span></label>}
        <label>{t("quota")}<Input value={quota} onChange={e => setQuota(e.target.value)} inputMode="decimal" /></label>
        <label>{t("time")}<Input type="datetime-local" value={when} onChange={e => setWhen(e.target.value)} /></label>
        <label>{t("reference")}<Input value={reference} onChange={e => setReference(e.target.value)} /></label>
        <label>{t("note")}<Input value={note} onChange={e => setNote(e.target.value)} /></label>
        <label className="flex gap-2"><input type="checkbox" checked={confirmed} onChange={e => setConfirmed(e.target.checked)} />{t("actualReceived")}</label>
      </div>}
      {originalPurchase && <div className="whitespace-pre-line rounded-control border border-hairline p-3 text-sm"><p>{t("registered", { id: originalPurchase.id })}</p><p>{summary(originalPurchase)}</p></div>}
      <div className="flex flex-wrap gap-3">
        {!rejected && <ConfirmButton title={t("complete")} description={summary(payload)} error={message} validate={validate} onConfirm={submit}>{t(operation ? "retry" : "complete")}</ConfirmButton>}
        {operation && <Button variant="outline" onClick={() => void lookup()}>{t("lookup")}</Button>}
        {rejected && <ConfirmButton title={t("endRejected")} description={t("endRejectedHint")} onConfirm={endRejected}>{t("endRejected")}</ConfirmButton>}
      </div>
    </div>}
    <Button variant="outline" onClick={() => void query.refetch()}>{t("refresh")}</Button>
    {query.isPending ? <p role="status">{t("loading")}</p> : query.isError ? <p role="alert">{t("readFailed")}</p> : query.data?.items.length ? <ul className="space-y-3">{query.data.items.map(p => <li key={p.id} className="rounded-control border border-hairline p-4 text-sm">
      <p className="font-medium">{t("status." + (p.status || "completed"))}</p>
      <p className="font-medium">{t("received")}: {formatPayMinor(p.cash_currency, p.cash_amount_minor)} {p.cash_currency} · {t("saleValue")}: {formatUsdMinor(p.sale_amount_minor)} USD · {t("delivered")}: {formatUsdMinor(p.quota_amount_minor)} USD</p>
      <p>{t("time")}: {new Date(p.occurred_at).toLocaleString(undefined, { timeZone: "Asia/Shanghai" })} · {t("completedAt")}: {new Date(p.completed_at).toLocaleString(undefined, { timeZone: "Asia/Shanghai" })}</p>
      {p.reversed_at && <p>{t("reversedAt")}: {new Date(p.reversed_at).toLocaleString(undefined, { timeZone: "Asia/Shanghai" })} · {p.reversal_reason} · {t("noCashRefund")}</p>}
      {p.external_reference && <p>{t("reference")}: {p.external_reference}</p>}<p className="break-all text-xs text-ink-mute">{p.id} · {p.oem_channel_org_id}</p>
      {!channel && canWrite("channels.quota", viewer) && <OEMPurchaseReversal original={p} onChanged={async () => { await Promise.allSettled([query.refetch(), client.invalidateQueries({ predicate: q => q.queryKey.some(k => typeof k === "string" && (k.includes("pnl") || k.includes("dashboard") || k.includes("billing-report") || k.includes("channel-quotas"))) })]); }} />}
    </li>)}</ul> : <p>{t("empty")}</p>}
    <div className="flex gap-2">{cursor && <Button variant="outline" onClick={() => setCursor("")}>{t("first")}</Button>}{query.data?.next_cursor && <Button variant="outline" onClick={() => setCursor(query.data!.next_cursor!)}>{t("next")}</Button>}</div>
  </section>;
}
