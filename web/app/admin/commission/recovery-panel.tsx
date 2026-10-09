"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { ConfirmDialog } from "@/components/confirm-button";
import { ListResourceView } from "@/components/console/list-resource-view";
import { formatUsdMinor, parseUsdToMinor } from "@/lib/money";
import { actualTime, localNow, person, useCommissionPage, type Person, type WorkflowPayload } from "./workflow-client";
import type { WorkspaceProps } from "./workspace";

type Receipt = { id: string; amount_minor: number; reference?: string; note?: string; actor_user_id?: string; actor_email?: string; occurred_at?: string; created_at: string };
export type Recovery = { id: string; user_id: string; settlement_id: string; amount_minor: number; recovered_minor: number; status: string; created_at: string; recipient?: Person; receipts: Receipt[] };
const money = (n: number) => `${formatUsdMinor(n)} USD`;
export function RecoveryPanel({ prefix, scope, params, update, writable, operation, version }: WorkspaceProps) {
  const t = useTranslations("commissionWorkflow");
  const [selected, setSelected] = useState<Recovery | null>(null);
  const [amount, setAmount] = useState("");
  const [occurred, setOccurred] = useState(localNow);
  const [reference, setReference] = useState("");
  const [note, setNote] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const [review, setReview] = useState<WorkflowPayload | null>(null);
  const trigger = useRef<HTMLButtonElement | null>(null);
  const query = new URLSearchParams(params); query.delete('tab'); query.delete('from'); query.delete('next'); query.set('limit', '30'); if (!query.has('status')) query.set('status', 'pending'); if (query.get('status') === 'all') query.delete('status');
  const list = useCommissionPage<Recovery>(scope, `${prefix}/commission-recoveries?${query}`);
  const locked = operation.busy || Boolean(operation.saved);
  const ready = ['ready', 'empty'].includes(list.snapshot.phase) && !list.refreshing;
  useEffect(() => { void list.reload(); }, [version, list.reload]);
  useEffect(() => { setSelected(null); setReview(null); }, [scope, params.get('channel_id')]);
  const remaining = selected ? selected.amount_minor - selected.recovered_minor : 0;
  const minor = parseUsdToMinor(amount), date = actualTime(occurred);
  const valid = minor !== null && minor > 0 && minor <= remaining && confirmed && date && new Date(date).getTime() <= Date.now() + 300000 && new TextEncoder().encode(reference.trim()).length <= 200 && new TextEncoder().encode(note.trim()).length <= 1000;
  return <section className="space-y-4 rounded-card border border-hairline bg-canvas-raised p-5" aria-label={t('tab.recovery')}>
    <div className="flex flex-wrap justify-between gap-3"><h2 className="text-lg font-semibold">{t('tab.recovery')}</h2><Button variant="outline" disabled={list.refreshing} onClick={() => void list.reload()}>{t('refresh')}</Button></div>
    <label className="text-sm">{t('statusLabel')} <select aria-label={t('statusLabel')} className="rounded-control border border-hairline bg-canvas p-2" value={params.get('status') || 'pending'} onChange={e => update({ status: e.target.value, cursor: '' })}><option value="pending">{t('status.pending')}</option><option value="closed">{t('status.closed')}</option><option value="all">{t('allStates')}</option></select></label>
    <ListResourceView snapshot={list.snapshot} emptyTitle={t('empty')} emptyDetail={t('emptyDetail')} onRetry={() => void list.reload()}>
      <ul className="grid gap-3">{list.snapshot.items.map(item => <li key={item.id} className="rounded-control border border-hairline p-4 text-sm"><p className="break-all font-medium">{person(item)}</p><p className="mt-2">{item.status === 'closed' ? t('status.closed') : t('status.pending')} · {t('remaining')}: {money(item.amount_minor - item.recovered_minor)}</p><p>{t('originalRecovery')}: {money(item.amount_minor)} · {t('recovered')}: {money(item.recovered_minor)}</p><p className="mt-1 break-all text-ink-secondary">{item.id} · {item.settlement_id}</p><Button className="mt-3" variant="outline" disabled={!ready} onClick={e => { trigger.current = e.currentTarget; setSelected(item); setAmount(String((item.amount_minor - item.recovered_minor) / 1_000_000)); setOccurred(localNow()); setConfirmed(false); setReference(''); setNote(''); }}>{writable && item.status === 'pending' ? t('recordRecovery') : t('detail')}</Button></li>)}</ul>
    </ListResourceView>
    {list.page.total !== undefined && <div className="flex flex-wrap items-center gap-3 text-sm"><span>{t('total', { count: list.page.total })}</span>{params.get('cursor') && <Button variant="outline" disabled={!ready} onClick={() => update({ cursor: '' })}>{t('firstPage')}</Button>}{list.page.next_cursor && <Button variant="outline" disabled={!ready} onClick={() => update({ cursor: list.page.next_cursor! })}>{t('nextPage')}</Button>}</div>}
    <Dialog open={Boolean(selected)} onOpenChange={open => { if (!open) setSelected(null); }}><DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto" onCloseAutoFocus={event => { event.preventDefault(); trigger.current?.focus(); }}><DialogTitle>{t('recordRecovery')}</DialogTitle><DialogDescription>{selected ? `${person(selected)} · ${t('remaining')}: ${money(remaining)}` : ''}</DialogDescription>
      {selected && <><p className="break-all text-sm">{selected.id} · {selected.settlement_id}</p><ul className="space-y-3 text-sm">{selected.receipts.map(receipt => <li key={receipt.id} className="break-all border-t border-hairline pt-2"><p>{money(receipt.amount_minor)} · {receipt.id}</p><p>{t('occurredAt')}: {receipt.occurred_at ? new Date(receipt.occurred_at).toLocaleString() : t('legacyTime')}</p><p>{t('recordedAt')}: {new Date(receipt.created_at).toLocaleString()} · {receipt.actor_email || receipt.actor_user_id || '—'}</p>{receipt.reference && <p>{t('reference')}: {receipt.reference}</p>}{receipt.note && <p>{t('note')}: {receipt.note}</p>}</li>)}</ul></>}
      {writable && selected?.status === 'pending' && <fieldset disabled={locked || !ready} className="space-y-3"><p className="text-sm">{t('recoveryHint')}</p><label className="block text-sm">{t('recoveryAmount')}<Input aria-label={t('recoveryAmount')} inputMode="decimal" value={amount} onChange={e => setAmount(e.target.value)} /></label><p className="text-sm text-ink-secondary">{t('amountBound', { amount: money(remaining) })}</p><label className="block text-sm">{t('occurredAt')}<Input aria-label={t('occurredAt')} type="datetime-local" value={occurred} onChange={e => setOccurred(e.target.value)} /></label><label className="block text-sm">{t('reference')}<Input value={reference} onChange={e => setReference(e.target.value)} /></label><label className="block text-sm">{t('note')}<Input value={note} onChange={e => setNote(e.target.value)} /></label><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={confirmed} onChange={e => setConfirmed(e.target.checked)} />{t('recoveryHappened')}</label><Button disabled={!valid} onClick={() => setReview({ kind: 'recovery', target: selected.id, path: `${prefix}/commission-recoveries/${encodeURIComponent(selected.id)}/receipts${params.get('channel_id') ? `?channel_id=${encodeURIComponent(params.get('channel_id')!)}` : ''}`, lookup: `${prefix}/commission-recovery-operations`, label: `${person(selected)} · ${money(minor!)}`, data: { amount_minor: minor, occurred_at: date, confirmed: true, reference: reference.trim(), note: note.trim() } })}>{t('reviewRecovery')}</Button></fieldset>}
    </DialogContent></Dialog>
    <ConfirmDialog open={Boolean(review)} onOpenChange={open => { if (!open) setReview(null); }} title={t('recordRecovery')} description={review ? `${review.label}\n${t('occurredAt')}: ${new Date(String(review.data.occurred_at)).toLocaleString()}\n${t('remaining')}: ${money(remaining - Number(review.data.amount_minor))}\n${t('recoveryConfirm')}` : ''} error={operation.error} onConfirm={async () => { if (!review) return false; const ok = await operation.run(review); if (ok) { setSelected(null); setReview(null); } return ok; }} />
  </section>;
}
