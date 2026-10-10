"use client";
import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { useViewer } from "@/components/rbac/viewer-context";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { formatUsdMinor } from "@/lib/money";
import { beginOperation, finishOperation, loadOperation, type SavedOperation } from "@/lib/stable-operation";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { ConfirmButton } from "@/components/confirm-button";

type Original = { id: string; oem_channel_org_id: string; quota_amount_minor: number; sale_amount_minor: number; status?: string };
type Payload = { purchase_id: string; reason: string };
export function OEMPurchaseReversal({ original, onChanged }: { original: Original; onChanged: () => Promise<unknown> }) {
  const t = useTranslations("oemPurchase"), viewer = useViewer();
  const key = `oem-purchase-reverse:${viewer.userId}:${original.oem_channel_org_id}:${original.id}`;
  const [operation, setOperation] = useState<SavedOperation<Payload> | null>(null);
  const [open, setOpen] = useState(false), [reason, setReason] = useState(""), [message, setMessage] = useState("");
  useEffect(() => { const op = loadOperation<Payload>(key); setOperation(op); setOpen(Boolean(op)); setReason(op?.payload.reason || ""); setMessage(""); }, [key]);
  async function completed() { finishOperation(key); setOperation(null); setOpen(false); setReason(""); setMessage(t("reversalDone")); await onChanged(); return true; }
  function validate() { const valid = Boolean(operation || reason.trim()); setMessage(valid ? "" : t("reasonRequired")); return valid; }
  async function submit() {
    if (!validate()) return false;
    try {
      const op = beginOperation(key, { purchase_id: original.id, reason: reason.trim() }); setOperation(op);
      const r = await fetch(`${apiBase}/admin/oem-purchases/${encodeURIComponent(original.id)}/reverse`, { method: "POST", credentials: "include", headers: confirmHeaders, body: JSON.stringify({ operation_id: op.id, reason: op.payload.reason }) }); const b = await r.json();
      if (r.ok && b.item?.status === "reversed" && b.item.reversal_operation_id === op.id && b.item.reversal_reason === op.payload.reason) return await completed();
      // A locked transaction's quota rejection proves no reversal took place.
      // Transport/5xx failures retain the original operation for lookup/retry.
      if (r.status >= 400 && r.status < 500 && ![408, 429].includes(r.status)) { finishOperation(key); setOperation(null); }
      setMessage(b.error?.message || t("reversalUnknown")); return false;
    } catch { setMessage(t("reversalUnknown")); return false; }
  }
  async function lookup() {
    if (!operation) return;
    try {
      const r = await fetch(`${apiBase}/admin/oem-purchases/${encodeURIComponent(original.id)}`, { credentials: "include" }); const b = await r.json();
      if (r.ok && b.item?.status === "reversed") {
        if (b.item.reversal_operation_id === operation.id && b.item.reversal_reason === operation.payload.reason) await completed();
        else { setMessage(t("alreadyReversed")); finishOperation(key); setOperation(null); setOpen(false); await onChanged(); }
      } else setMessage(b.error?.message || t("reversalUnknown"));
    } catch { setMessage(t("reversalUnknown")); }
  }
  if (original.status === "reversed" && !operation) return message ? <p role="status">{message}</p> : null;
  return <div className="mt-3 space-y-3 border-t border-hairline pt-3">
    {message && <p role="status">{message}</p>}
    {!open ? <Button variant="outline" onClick={() => setOpen(true)}>{t("reverse")}</Button> : <>
      <p>{t("reverseHint")}</p><label>{t("reversalReason")}<Input disabled={Boolean(operation)} value={reason} onChange={e => setReason(e.target.value)} /></label>
      {operation && <p role="alert">{t("reversalUnknown")} {t("original", { id: operation.id })} · {operation.payload.reason}</p>}
      <div className="flex flex-wrap gap-3"><ConfirmButton title={t("reverse")} description={t("reversalConfirm", { id: original.id, owner: original.oem_channel_org_id, quota: formatUsdMinor(original.quota_amount_minor), sale: formatUsdMinor(original.sale_amount_minor), reason: operation?.payload.reason || reason })} error={message} validate={validate} onConfirm={submit}>{t(operation ? "retryReverse" : "reverse")}</ConfirmButton>
        {operation ? <Button variant="outline" onClick={() => void lookup()}>{t("lookup")}</Button> : <Button variant="outline" onClick={() => setOpen(false)}>{t("cancelReverse")}</Button>}
      </div>
    </>}
  </div>;
}
