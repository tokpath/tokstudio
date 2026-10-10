"use client";

import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { useViewer } from "@/components/rbac/viewer-context";
import { ConfirmButton } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { formatUsdMinor, parseUsdToMinor } from "@/lib/money";
import { beginOperation, finishOperation, loadOperation, type SavedOperation } from "@/lib/stable-operation";
import { OEMPurchasesPanel } from "@/components/oem-purchases";

type Quota = { available_minor: number; issued_minor: number; consumed_minor: number };
type Payload = { channel_org_id: string; amount_minor: number; preview_before_minor: number };
export function ChannelQuotaPanel({ channelID, channelType }: { channelID: string; channelType: string }) {
  const t = useTranslations("serviceQuota");
  const viewer = useViewer();
  const queryClient = useQueryClient();
  const storageKey = `quota-adjust:${viewer.userId}:${channelID}`;
  const [amount, setAmount] = useState("");
  const [ratio, setRatio] = useState("");
  const [message, setMessage] = useState("");
  const [operation, setOperation] = useState<SavedOperation<Payload> | null>(null);
  const path = `/admin/channel-quotas/${encodeURIComponent(channelID)}`;
  const query = useQuery({ queryKey: [viewer.userId, path], enabled: channelType === "C", queryFn: async () => {
    const response = await fetch(`${apiBase}${path}`, { credentials: "include" }); const body = await response.json();
    if (response.status === 404) return { quota: { available_minor: 0, issued_minor: 0, consumed_minor: 0 } as Quota, exists: false };
    if (!response.ok || !body.quota) throw new Error(body.error?.message || t("readFailed"));
    return { quota: body.quota as Quota, exists: true };
  }});
  const rule = useQuery({ queryKey: [viewer.userId, path, "issue-rule"], enabled: channelType === "C", queryFn: async () => {
    const response = await fetch(`${apiBase}${path}/issue-rule`, { credentials: "include" }); const body = await response.json();
    if (!response.ok || !body.rule) throw new Error(t("readFailed")); return body.rule as { issue_ratio_bps: number };
  }});
  useEffect(() => { if (rule.data) setRatio(String(rule.data.issue_ratio_bps / 100)); }, [rule.data]);
  useEffect(() => { const saved = loadOperation<Payload>(storageKey); setOperation(saved); setAmount(saved ? String(saved.payload.amount_minor / 1_000_000) : ""); setMessage(saved ? t("unconfirmed") : ""); }, [storageKey, t]);
  const minor = operation?.payload.amount_minor ?? parseUsdToMinor(amount);
  const current = query.data?.quota.available_minor;
  function validate() {
    if (operation) return true;
    if (query.isPending || query.isError || !viewer.userId || minor == null || minor === 0 || current == null || !Number.isSafeInteger(current + minor) || current + minor < 0) { setMessage(t("invalid")); return false; }
    setMessage(""); return true;
  }
  async function adjust() {
    if (!validate()) return false;
    try {
      const pending = beginOperation(storageKey, { channel_org_id: channelID, amount_minor: minor!, preview_before_minor: current! }); setOperation(pending);
      const response = await fetch(`${apiBase}/admin/channel-quotas/grant`, { method: "POST", credentials: "include", headers: confirmHeaders, body: JSON.stringify({ ...pending.payload, operation_id: pending.id }) }); const body = await response.json();
      if (!response.ok) {
        if (response.status >= 400 && response.status < 500 && ![408, 409, 429].includes(response.status)) { finishOperation(storageKey); setOperation(null); }
        setMessage(body.error?.message || t("unconfirmed")); return false;
      }
      finishOperation(storageKey); setOperation(null); setAmount("");
      const result = await query.refetch();
      setMessage(result.isError ? t("savedReadFailed") : t("saved", { id: body.operation?.id || pending.id }));
      await queryClient.invalidateQueries({ predicate: query => query.queryKey.some(key => String(key).includes("/admin/oem-deliveries")) });
      return true;
    } catch { setMessage(t("unconfirmed")); return false; }
  }
  async function lookup() {
    if (!operation) return;
    try {
      const response = await fetch(`${apiBase}${path}/operations?operation_id=${encodeURIComponent(operation.id)}`, { credentials: "include" }); const body = await response.json();
      if (response.ok && body.operation) { finishOperation(storageKey); setOperation(null); setAmount(""); await query.refetch(); setMessage(t("saved", { id: body.operation.id })); }
      else setMessage(t("unconfirmed"));
    } catch { setMessage(t("readFailed")); }
  }
  async function saveRatio() {
    const bps = Math.round(Number(ratio) * 100);
    if (!/^\d+(\.\d{1,2})?$/.test(ratio.trim()) || !Number.isSafeInteger(bps) || bps < 1000 || bps > 100000 || !rule.data || rule.isError) { setMessage(t("invalidRatio")); return false; }
    try { const response = await fetch(`${apiBase}${path}/issue-rule`, { method: "PATCH", credentials: "include", headers: confirmHeaders, body: JSON.stringify({ issue_ratio_bps: bps }) }); const body = await response.json(); if (!response.ok) { setMessage(body.error?.message || t("readFailed")); return false; } await rule.refetch(); setMessage(t("ruleSaved")); return true; } catch { setMessage(t("unconfirmed")); return false; }
  }
  if (channelType !== "C") return null;
  return <div className="space-y-5"><OEMPurchasesPanel ownerID={channelID} allowCreate/><details className="rounded-card border border-hairline bg-canvas-raised p-6"><summary className="text-lg font-semibold">{t("title")}</summary><p className="mt-2 text-sm text-ink-secondary">{t("hint")}</p>
    {message ? <p role="status" className="mt-3 text-sm">{message}</p> : null}
    {query.isPending ? <p role="status">{t("loading")}</p> : query.isError ? <p role="alert">{t("readFailed")}</p> : <p className="my-3">{t("available", { amount: formatUsdMinor(current) })}{!query.data?.exists ? ` · ${t("newPool")}` : ""}</p>}
    <div className="my-3 flex flex-wrap gap-3"><Input disabled={Boolean(operation)} aria-label={t("amount")} placeholder={t("amount")} value={amount} onChange={event => setAmount(event.target.value)} /><Button variant="outline" onClick={() => void query.refetch()}>{t("refresh")}</Button><ConfirmButton disabled={viewer.loading || (!operation && (query.isPending || query.isError))} title={t("adjust")} description={t("confirm", { amount: formatUsdMinor(minor), before: formatUsdMinor(operation?.payload.preview_before_minor ?? current), after: formatUsdMinor((operation?.payload.preview_before_minor ?? current) != null && minor != null ? (operation?.payload.preview_before_minor ?? current)! + minor : undefined) })} validate={validate} onConfirm={adjust}>{operation ? t("retry") : t("adjust")}</ConfirmButton>{operation ? <Button variant="outline" onClick={() => void lookup()}>{t("lookup")}</Button> : null}</div>
    <details className="mt-4"><summary>{t("ratioTitle")}</summary><p className="my-3 text-sm">{t("ratioHint")}</p>{rule.isError ? <p role="alert">{t("readFailed")}</p> : null}<div className="flex gap-3"><Input disabled={rule.isPending || rule.isError} aria-label={t("ratio")} value={ratio} onChange={event => setRatio(event.target.value)} /><ConfirmButton disabled={rule.isPending || rule.isError} title={t("saveRatio")} description={t("ratioConfirm", { before: rule.data ? rule.data.issue_ratio_bps / 100 : "—", after: ratio || "—" })} validate={() => Boolean(rule.data) && Number(ratio)>=10 && Number(ratio)<=1000 && /^\d+(\.\d{1,2})?$/.test(ratio.trim())} onConfirm={saveRatio}>{t("saveRatio")}</ConfirmButton></div></details>
  </details></div>;
}
