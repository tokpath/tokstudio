"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { beginOperation, finishOperation, loadOperation, type SavedOperation } from "@/lib/stable-operation";
import { useListResource } from "@/hooks/use-list-resource";
import type { ListLoadResult } from "@/lib/list-resource";
import { initialListSnapshot } from "@/lib/list-resource";
import { Button } from "@/components/ui/button";

export type Person = { email?: string; display_name?: string };
export type Context = { owner_id: string; owner_code?: string; channel_ids: string[]; channel_codes: Record<string, string> };
export type WorkflowPayload = { kind: "settle" | "payout" | "recovery" | "unfreeze"; target: string; path: string; lookup: string; label: string; data: Record<string, unknown> };
export type Body = { item?: Record<string, unknown>; items?: unknown[]; unfrozen?: number; operation_id?: string; error?: { code?: string; message?: string } };
export function person(item: { recipient?: Person; beneficiary_role_id?: string; user_id?: string }) {
  return item.recipient?.email ? `${item.recipient.display_name ? `${item.recipient.display_name} · ` : ""}${item.recipient.email}` : item.beneficiary_role_id || item.user_id || "—";
}
export function localNow() { const now = new Date(); return new Date(now.getTime() - now.getTimezoneOffset() * 60000).toISOString().slice(0, 16); }
export function actualTime(value: string) { const date = new Date(value); return Number.isFinite(date.getTime()) ? date.toISOString() : ""; }

export function useCommissionPage<T>(scope: string, path: string) {
  const [page, setPage] = useState<{ total?: number; next_cursor?: string }>({});
  const [acceptedKey, setAcceptedKey] = useState("");
  const list = useListResource<T>({ queryKey: `${scope}:${path}`, enabled: Boolean(scope), load: async (): Promise<ListLoadResult<T>> => {
    try {
      const response = await fetch(`${apiBase}${path}`, { credentials: "include" });
      const body = await response.json();
      return { ok: response.ok, status: response.status, items: Array.isArray(body.items) ? body.items : [], message: body.error?.message, code: body.error?.code, extras: body };
    } catch { return { ok: false, network: true }; }
  }, onAccepted: result => { setAcceptedKey(`${scope}:${path}`); if (result.ok) setPage(result.extras as typeof page); else if (result.status === 401 || result.status === 403) setPage({}); } });
  useEffect(() => setPage({}), [scope, path]);
  return { ...list, snapshot: acceptedKey === `${scope}:${path}` ? list.snapshot : initialListSnapshot<T>(), page: acceptedKey === `${scope}:${path}` ? page : {} };
}

export function useCommissionOperation(scope: string, onRecorded: () => void) {
  const t = useTranslations("commissionWorkflow");
  const key = scope ? `commission-operation:${scope}` : "";
  const keyRef = useRef(key); keyRef.current = key;
  const recordedRef = useRef(onRecorded); recordedRef.current = onRecorded;
  const [saved, setSaved] = useState<SavedOperation<WorkflowPayload> | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  useEffect(() => { setSaved(key ? loadOperation<WorkflowPayload>(key) : null); setError(""); setMessage(""); setBusy(false); }, [key]);
  function complete(operationKey: string, op: SavedOperation<WorkflowPayload>) {
    if (keyRef.current !== operationKey) return;
    finishOperation(operationKey); setSaved(null); setError(""); setMessage(t("recorded", { label: op.payload.label }));
    // A subsequent refresh failure never changes this confirmed receipt.
    recordedRef.current();
  }
  async function run(payload?: WorkflowPayload) {
    if (!key || busy) return false;
    const operationKey = key;
    let op: SavedOperation<WorkflowPayload>;
    let existed = false;
    try {
      const original = loadOperation<WorkflowPayload>(key);
      existed = Boolean(original);
      op = original || (payload ? beginOperation(key, payload) : null)!;
      if (!op) return false;
      setSaved(op);
    } catch { setError(t("storageFailed")); return false; }
    setBusy(true); setError("");
    try {
      const response = await fetch(`${apiBase}${op.payload.path}`, { method: "POST", credentials: "include", headers: confirmHeaders, body: JSON.stringify({ ...op.payload.data, [op.payload.kind === "recovery" ? "idempotency_key" : "operation_id"]: op.id }) });
      const body: Body = await response.json();
      if (keyRef.current !== operationKey) return false;
      if (!response.ok) {
        // Only a first, explicit validation rejection permits a new draft.
        // A stored original operation survives later conflicts and temporary 404s.
        if (!existed && [400, 409].includes(response.status)) { finishOperation(operationKey); setSaved(null); }
        setError(body.error?.message || t("unknown")); return false;
      }
      const valid = op.payload.kind === "settle" ? Array.isArray(body.items) && body.operation_id === op.id : op.payload.kind === "unfreeze" ? typeof body.unfrozen === "number" : op.payload.kind === "payout" ? body.item?.id === op.payload.target && body.item?.status === "paid" : typeof body.item?.id === "string" && body.item.recovery_id === op.payload.target;
      if (!valid) { setError(t("unknown")); return false; }
      complete(operationKey, op); return true;
    } catch { if (keyRef.current === operationKey) setError(t("unknown")); return false; }
    finally { if (keyRef.current === operationKey) setBusy(false); }
  }
  async function check() {
    if (!key || !saved || busy) return;
    const operationKey = key, op = saved;
    setBusy(true); setError("");
    try {
      const response = await fetch(`${apiBase}${op.payload.lookup}/${encodeURIComponent(op.id)}${op.payload.path.includes("?") ? `?${op.payload.path.split("?")[1]}` : ""}`, { credentials: "include" });
      const body: Body = await response.json();
      if (keyRef.current !== operationKey) return;
      const valid = op.payload.kind === "recovery" ? typeof body.item?.id === "string" && body.operation_id === op.id && body.item.recovery_id === op.payload.target : body.item?.operation_id === op.id && body.item?.kind === op.payload.kind && body.item.result != null;
      if (response.ok && valid) complete(operationKey, op);
      else setError(response.status === 404 ? t("notFoundYet") : body.error?.message || t("unknown"));
    } catch { if (keyRef.current === operationKey) setError(t("unknown")); }
    finally { if (keyRef.current === operationKey) setBusy(false); }
  }
  return { saved, busy, error, message, run, check };
}
export type OperationController = ReturnType<typeof useCommissionOperation>;
export function OperationStatus({ operation }: { operation: OperationController }) {
  const t = useTranslations("commissionWorkflow");
  return <div aria-live="polite" className="space-y-2 text-sm">
    {operation.message && <p role="status">{operation.message}</p>}
    {operation.error && <p role="alert" className="text-danger">{operation.error}</p>}
    {operation.saved && <section className="rounded-control border border-hairline p-4" aria-label={t("pendingOperation")}><p>{t("pendingOperation")}: {operation.saved.payload.label}</p><p className="mt-1 break-all text-ink-secondary">{operation.saved.id}</p><div className="mt-3 flex flex-wrap gap-2"><Button variant="outline" disabled={operation.busy} onClick={() => void operation.check()}>{t("checkOriginal")}</Button><Button disabled={operation.busy} onClick={() => void operation.run()}>{t("retryOriginal")}</Button></div></section>}
  </div>;
}
