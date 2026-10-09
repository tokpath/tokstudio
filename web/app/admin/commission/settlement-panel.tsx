"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { apiBase } from "@/lib/api";
import { formatUsdMinor } from "@/lib/money";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { ListResourceView } from "@/components/console/list-resource-view";
import { ConfirmDialog } from "@/components/confirm-button";
import { actualTime, localNow, person, useCommissionPage, type Person, type WorkflowPayload } from "./workflow-client";
import type { WorkspaceProps } from "./workspace";

export type Settlement = { id: string; amount_minor: number; status: string; created_at: string; period_start: string; period_end: string; channel_code?: string; channel_org_id?: string; beneficiary_role_id?: string; recipient?: Person; payout_id?: string; payout_reference?: string; payout_note?: string; payout_occurred_at?: string; payout_recorded_at?: string; reversed_minor?: number; recovered_minor?: number; recovery_pending_minor?: number; entries_snapshot_complete?: boolean };
type Entry = { id: string; kind: string; amount_minor: number; status: string; created_at: string; available_at?: string; recipient?: Person; beneficiary_role_id?: string; channel_code?: string; channel_org_id?: string; settlement_id?: string; usage_event_id?: string; request_id?: string; policy_version?: string };
type Preview = { id: string; owner_id: string; period_start: string; period_end: string; fact_start?: string; fact_end?: string; entry_count: number; recipient_count: number; settlement_count: number; amount_minor: number; min_settle_minor: number; ignore_minimum: boolean; policy_version: string; groups: { channel_org_id: string; beneficiary_role_id: string; entry_count: number; amount_minor: number }[]; excluded: Record<string, { entry_count: number; amount_minor: number }>; expires_at: string };
type PreviewBody = { preview: Preview; recipients: Record<string, Person>; channel_codes: Record<string, string> };
const money = (value: number) => `${formatUsdMinor(value)} USD`;
export function SettlementPanel(props: WorkspaceProps & { view: "pending" | "payout" | "all" }) {
  const { prefix, scope, context, params, update, writable, operation, version, view } = props;
  const t = useTranslations("commissionWorkflow");
  const [ignore, setIgnore] = useState(false);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [preview, setPreview] = useState<PreviewBody | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewError, setPreviewError] = useState("");
  const [selected, setSelected] = useState<Settlement | null>(null);
  const [detail, setDetail] = useState<{ item: Settlement; entries: Entry[] } | null>(null);
  const [detailError, setDetailError] = useState("");
  const [occurred, setOccurred] = useState(localNow);
  const [confirmed, setConfirmed] = useState(false);
  const [reference, setReference] = useState("");
  const [note, setNote] = useState("");
  const [review, setReview] = useState<WorkflowPayload | null>(null);
  const trigger = useRef<HTMLButtonElement | null>(null);
  const query = new URLSearchParams(params); query.delete("tab"); query.delete("from"); query.delete("next"); query.set("limit", "30");
  if (view === "payout") query.set("status", "settled");
  if (view === "pending" && !query.has("status")) query.set("status", "available");
  const selectedScope = params.get("channel_id") ? `?channel_id=${encodeURIComponent(params.get("channel_id")!)}` : "";
  const path = `${prefix}/${view === "payout" ? `settlements${prefix === '/channel' ? '/manage' : ''}` : "commissions"}?${query}`;
  const list = useCommissionPage<Entry | Settlement>(scope, path);
  const scopeRef = useRef(scope); scopeRef.current = scope;
  const detailSeq = useRef(0), previewSeq = useRef(0);
  useEffect(() => { void list.reload(); }, [version, list.reload]);
  useEffect(() => { setSelected(null); setDetail(null); setPreview(null); setPreviewOpen(false); setReview(null); ++detailSeq.current; ++previewSeq.current; }, [scope, params.get('channel_id'), view]);
  const ready = ['ready', 'empty'].includes(list.snapshot.phase) && !list.refreshing;
  const locked = operation.busy || Boolean(operation.saved);
  function payload(kind: WorkflowPayload["kind"], target: string, path: string, data: Record<string, unknown>, label: string): WorkflowPayload {
    return { kind, target, path: `${prefix}${path}${selectedScope}`, data, label, lookup: `${prefix}/commission-operations` };
  }
  async function loadPreview() {
    setPreviewOpen(true); setPreview(null); setPreviewError(""); setPreviewLoading(true);
    const seq = ++previewSeq.current, original = scope;
    try {
      const q = new URLSearchParams(selectedScope.slice(1)); if (ignore) q.set("ignore_minimum", "1");
      const response = await fetch(`${apiBase}${prefix}/commissions/settlement-preview?${q}`, { credentials: "include" }); const body = await response.json();
      if (!response.ok || !body.preview?.id) throw new Error(body.error?.message || t("loadFailed"));
      if (seq === previewSeq.current && scopeRef.current === original) setPreview(body);
    } catch (e) { if (seq === previewSeq.current && scopeRef.current === original) setPreviewError(e instanceof Error ? e.message : t("loadFailed")); }
    finally { if (seq === previewSeq.current && scopeRef.current === original) setPreviewLoading(false); }
  }
  async function select(item: Settlement) {
    const seq = ++detailSeq.current, original = scope;
    setSelected(item); setDetail(null); setDetailError(""); setOccurred(localNow()); setConfirmed(false); setReference(""); setNote("");
    try {
      const response = await fetch(`${apiBase}${prefix}/settlements/${encodeURIComponent(item.id)}${selectedScope}`, { credentials: "include" }); const body = await response.json();
      if (!response.ok || !body.item?.id || !Array.isArray(body.entries)) throw new Error(body.error?.message || t("loadFailed"));
      if (seq === detailSeq.current && scopeRef.current === original) setDetail(body);
    } catch (e) { if (seq === detailSeq.current && scopeRef.current === original) setDetailError(e instanceof Error ? e.message : t("loadFailed")); }
  }
  const selectedID = params.get('settlement_id');
  useEffect(() => { if (selectedID) void select({ id: selectedID, amount_minor: 0, status: '', created_at: '', period_start: '', period_end: '' }); }, [scope, selectedID, selectedScope]);
  const current = detail?.item || selected;
  const date = actualTime(occurred);
  const valid = confirmed && date && new Date(date).getTime() <= Date.now() + 300000 && new TextEncoder().encode(reference.trim()).length <= 200 && new TextEncoder().encode(note.trim()).length <= 1000;
  const statusName = (status: string) => t.has(`status.${status}`) ? t(`status.${status}`) : status;
  const next = list.page.next_cursor;
  return <section className="space-y-4 rounded-card border border-hairline bg-canvas-raised p-5" aria-label={t(`tab.${view}`)}>
    <div className="flex flex-wrap items-center justify-between gap-3"><h2 className="text-lg font-semibold">{t(`tab.${view}`)}</h2><Button variant="outline" onClick={() => void list.reload()} disabled={list.refreshing}>{t("refresh")}</Button></div>
    <div className="flex flex-wrap items-center gap-3">
      {view !== 'payout' && <label className="text-sm">{t("statusLabel")} <select className="rounded-control border border-hairline bg-canvas p-2" aria-label={t("statusLabel")} value={params.get('status') || (view === 'pending' ? 'available' : '')} onChange={e => update({ status: e.target.value, cursor: '' })}><option value="">{t("allStates")}</option>{(view === 'pending' ? ['available','frozen','held'] : ['available','frozen','held','settled','paid','reversed']).map(status => <option key={status} value={status}>{statusName(status)}</option>)}</select></label>}
      {view === 'pending' && writable && <><Button variant="outline" disabled={!ready || locked} onClick={() => setReview(payload("unfreeze", "", "/commissions/unfreeze", {}, t("unfreeze")))}>{t("unfreeze")}</Button><Button disabled={!ready || locked} onClick={() => void loadPreview()}>{t("preview")}</Button></>}
    </div>
    <ListResourceView snapshot={list.snapshot} emptyTitle={t("empty")} emptyDetail={t("emptyDetail")} onRetry={() => void list.reload()}>
      <ul className="grid gap-3">{list.snapshot.items.map(item => <li key={item.id} className="rounded-control border border-hairline p-4 text-sm">
        <div className="flex flex-wrap justify-between gap-2"><p className="break-all font-medium">{person(item)}</p><span className="font-mono">{money(item.amount_minor)}</span></div>
        <p className="mt-1">{statusName(item.status)} · {item.channel_code || item.channel_org_id || '—'}</p>
        <p className="mt-1 break-all text-ink-secondary">{item.id} · {new Date(item.created_at).toLocaleString()}</p>
        {'available_at' in item && item.available_at && <p className="mt-1">{t("availableAt")}: {new Date(item.available_at).toLocaleString()}</p>}
        {view === 'payout' ? <Button variant="outline" className="mt-3" disabled={!ready} onClick={e => { trigger.current = e.currentTarget; void select(item as Settlement); }}>{writable ? t("recordPayout") : t("detail")}</Button> : <p className="mt-1 break-all text-ink-secondary">{(item as Entry).request_id || (item as Entry).usage_event_id || '—'}{(item as Entry).settlement_id && ` · ${(item as Entry).settlement_id}`}</p>}
      {view !== 'payout' && (item as Entry).settlement_id && <Button variant="outline" className="mt-3" onClick={e=>{ trigger.current=e.currentTarget; void select({id:(item as Entry).settlement_id!, amount_minor:0, status:'',created_at:'',period_start:'',period_end:''}); }}>{t('detail')}</Button>}
      </li>)}</ul>
    </ListResourceView>
    {list.page.total !== undefined && <div className="flex flex-wrap items-center gap-3 text-sm"><span>{t("total", { count: list.page.total })}</span>{params.get('cursor') && <Button variant="outline" disabled={!ready} onClick={() => update({ cursor: '' })}>{t("firstPage")}</Button>}{next && <Button variant="outline" disabled={!ready} onClick={() => update({ cursor: next })}>{t("nextPage")}</Button>}</div>}
    <Dialog open={previewOpen} onOpenChange={setPreviewOpen}><DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto"><DialogTitle>{t("preview")}</DialogTitle><DialogDescription>{t("previewHint")}</DialogDescription>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" disabled={locked} checked={ignore} onChange={e => { setIgnore(e.target.checked); setPreview(null); }} />{t("ignoreMinimum")}</label>
      {previewLoading && <p role="status">{t("loading")}</p>}{previewError && <p role="alert" className="text-danger">{previewError}</p>}
      {preview && <div className="space-y-3 text-sm"><p>{t("brand")}: {context.owner_code || context.owner_id} · {t("accountingPeriod")}: {preview.preview.period_start.slice(0, 7)}</p><p>{t("factRange")}: {preview.preview.fact_start ? new Date(preview.preview.fact_start).toLocaleString() : '—'} → {preview.preview.fact_end ? new Date(preview.preview.fact_end).toLocaleString() : '—'}</p><p>{t("previewCounts", { entries: preview.preview.entry_count, people: preview.preview.recipient_count, settlements: preview.preview.settlement_count })} · {money(preview.preview.amount_minor)}</p><p>{t("minimum")}: {money(preview.preview.min_settle_minor)} · {preview.preview.ignore_minimum ? t("minimumIgnored") : t("minimumApplied")}</p>
        <ul className="space-y-2">{preview.preview.groups.map(group => <li key={`${group.channel_org_id}:${group.beneficiary_role_id}`} className="border-t border-hairline pt-2">{person({ recipient: preview.recipients[group.beneficiary_role_id], beneficiary_role_id: group.beneficiary_role_id })} · {preview.channel_codes[group.channel_org_id] || group.channel_org_id} · {group.entry_count} · {money(group.amount_minor)}</li>)}</ul>
        <ul>{Object.entries(preview.preview.excluded).map(([status, fact]) => fact.entry_count > 0 && <li key={status}>{t("excluded")}: {statusName(status)} · {fact.entry_count} · {money(fact.amount_minor)}</li>)}</ul>
        <p className="text-ink-secondary">{t("policyVersion")}: {preview.preview.policy_version} · {t("expires")}: {new Date(preview.preview.expires_at).toLocaleTimeString()}</p></div>}
      <div className="flex flex-wrap gap-2"><Button variant="outline" disabled={previewLoading || locked} onClick={() => void loadPreview()}>{t("refreshPreview")}</Button><Button disabled={!preview || preview.preview.entry_count === 0 || locked || previewLoading} onClick={() => { if (preview) setReview(payload('settle', preview.preview.id, '/commissions/settle', { preview_id: preview.preview.id }, t('createSettlement'))); }}>{t("createSettlement")}</Button></div>
    </DialogContent></Dialog>
    <Dialog open={Boolean(selected)} onOpenChange={open => { if (!open) { setSelected(null); ++detailSeq.current; if (selectedID) update({settlement_id:''}); } }}><DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto" onCloseAutoFocus={event => { event.preventDefault(); trigger.current?.focus(); }}><DialogTitle>{t("detail")}</DialogTitle><DialogDescription>{detail ? `${person(detail.item)} · ${money(detail.item.amount_minor)}` : t('loading')}</DialogDescription>
      {detailError ? <div role="alert"><p>{detailError}</p><Button variant="outline" onClick={() => selected && void select(selected)}>{t("retry")}</Button></div> : !detail ? <p role="status">{t("loading")}</p> : <div className="space-y-3 text-sm"><p>{current?.id} · {statusName(current!.status)}</p><p>{t("originalAmount")}: {money(current!.amount_minor)} · {t("reversed")}: {money(current!.reversed_minor || 0)}</p>{current?.payout_id && <p className="break-all">{t("payoutRecord")}: {current.payout_id} · {t("occurredAt")}: {current.payout_occurred_at ? new Date(current.payout_occurred_at).toLocaleString() : t("legacyTime")} · {t("recordedAt")}: {current.payout_recorded_at ? new Date(current.payout_recorded_at).toLocaleString() : '—'}{current.payout_reference && ` · ${current.payout_reference}`}{current.payout_note && ` · ${current.payout_note}`}</p>}{!current?.entries_snapshot_complete && <p>{t("legacyEntries")}</p>}<ul className="space-y-2">{detail.entries.map(entry => <li key={entry.id} className="break-all border-t border-hairline pt-2">{entry.id} · {money(entry.amount_minor)} · {statusName(entry.status)}<p>{entry.usage_event_id || '—'} · {entry.policy_version}</p></li>)}</ul></div>}
      {writable && detail?.item.status === 'settled' && <fieldset disabled={locked} className="space-y-3"><p className="text-sm">{t("payoutHint")}</p><label className="block text-sm">{t("occurredAt")}<Input aria-label={t("occurredAt")} type="datetime-local" value={occurred} onChange={e => setOccurred(e.target.value)} /></label><label className="block text-sm">{t("reference")}<Input value={reference} onChange={e => setReference(e.target.value)} /></label><label className="block text-sm">{t("note")}<Input value={note} onChange={e => setNote(e.target.value)} /></label><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={confirmed} onChange={e => setConfirmed(e.target.checked)} />{t("payoutHappened")}</label><Button disabled={!valid} onClick={() => setReview(payload('payout', detail.item.id, `/settlements/${encodeURIComponent(detail.item.id)}/payout`, { method: 'manual', amount_minor: detail.item.amount_minor, occurred_at: date, confirmed: true, reference: reference.trim(), note: note.trim() }, `${person(detail.item)} · ${money(detail.item.amount_minor)}`))}>{t("reviewPayout")}</Button></fieldset>}
    </DialogContent></Dialog>
    <ConfirmDialog open={Boolean(review)} onOpenChange={open => { if (!open) setReview(null); }} title={review?.kind === 'payout' ? t('recordPayout') : review?.kind === 'unfreeze' ? t('unfreeze') : t('createSettlement')} description={review ? `${review.label}\n${review.kind === 'payout' ? `${t('occurredAt')}: ${new Date(String(review.data.occurred_at)).toLocaleString()}\n${t('payoutConfirm')}` : review.kind === 'unfreeze' ? t('unfreezeConfirm') : t('settleConfirm')}` : ''} error={operation.error} onConfirm={async () => { if (!review) return false; const ok = await operation.run(review); if (ok) { setSelected(null); setPreviewOpen(false); setReview(null); } return ok; }} />
  </section>;
}
