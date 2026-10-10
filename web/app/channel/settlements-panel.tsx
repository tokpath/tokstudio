"use client";
import { useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import { useTranslations } from 'next-intl';
import { useViewer } from '@/components/rbac/viewer-context';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Dialog, DialogContent, DialogTitle, DialogDescription } from '@/components/ui/dialog';
import { ListResourceView } from '@/components/console/list-resource-view';
import { useCommissionPage, person } from '@/app/admin/commission/workflow-client';
import type { Settlement } from '@/app/admin/commission/settlement-panel';
import { apiBase } from '@/lib/api';
import { formatUsdMinor } from '@/lib/money';
import { safeReturnHref } from '@/lib/return-context';
type Detail = { item: Settlement; entries: {id:string;amount_minor:number;status:string;usage_event_id:string;policy_version:string}[] };
const money = (n:number) => `${formatUsdMinor(n)} USD`;
export function ChannelCommissionRecords({ settlements = false }: { settlements?: boolean }) {
  const viewer = useViewer(), t = useTranslations('commissionWorkflow');
  const [params, setParams] = useState(new URLSearchParams()), [search, setSearch] = useState('');
  const [detail,setDetail]=useState<Detail|null>(null),[error,setError]=useState(''),[retry,setRetry]=useState(0);
  const trigger=useRef<HTMLButtonElement|null>(null);
  useEffect(() => { const read = () => { const p = new URLSearchParams(window.location.search); setParams(p); setSearch(p.get('q') || ''); }; read(); window.addEventListener('popstate',read); return () => window.removeEventListener('popstate',read); }, []);
  function update(values: Record<string,string>) { const p = new URLSearchParams(window.location.search); Object.entries(values).forEach(([key,v])=>v?p.set(key,v):p.delete(key)); window.history.pushState(null,'',`${window.location.pathname}${p.size?`?${p}`:''}`); setParams(p); }
  const q = new URLSearchParams(params); ['tab','from','next','return_to','settlement_id'].forEach(key=>q.delete(key)); q.set('limit','30');
  const scope = viewer.userId ? `${viewer.userId}:${typeof window === 'undefined' ? '' : window.location.host}:channel-self` : '';
  const list = useCommissionPage<Settlement>(scope, `/channel/${settlements ? 'settlements' : 'commissions'}?${q}`);
  const selectedID=params.get('settlement_id');
  useEffect(()=>{setDetail(null);setError('');if(!selectedID||!scope)return;let active=true;void fetch(`${apiBase}/channel/settlements/${encodeURIComponent(selectedID)}`,{credentials:'include'}).then(async response=>{const body=await response.json();if(!response.ok||body.item?.id!==selectedID||!Array.isArray(body.entries))throw new Error(body.error?.message||t('loadFailed'));if(active)setDetail(body);}).catch(e=>{if(active)setError(e instanceof Error?e.message:t('loadFailed'));});return()=>{active=false;};},[scope,selectedID,retry,t]);
  const statusName=(status:string)=>t.has(`status.${status}`)?t(`status.${status}`):status;
  return <section className="space-y-4 rounded-card border border-hairline bg-canvas-raised p-5">
    {params.has('return_to')&&<Link className="text-sm text-brand-emphasis underline" href={safeReturnHref(params.get('return_to'),'/channel/settlements')}>{t('returnTask')}</Link>}
    <h2 className="text-lg font-semibold">{t(settlements ? 'mySettlements' : 'tab.all')}</h2>
    <form className="flex flex-wrap gap-2" onSubmit={e=>{e.preventDefault();update({q:search.trim(),cursor:''});}}><Input aria-label={t('search')} placeholder={t('searchHint')} value={search} onChange={e=>setSearch(e.target.value)}/><Button variant="outline">{t('search')}</Button><Button type="button" variant="outline" onClick={()=>void list.reload()}>{t('refresh')}</Button></form>
    <ListResourceView snapshot={list.snapshot} emptyTitle={t('empty')} emptyDetail={t('emptyDetail')} onRetry={()=>void list.reload()}><ul className="grid gap-3">{list.snapshot.items.map(item=><li className="rounded-control border border-hairline p-4 text-sm" key={item.id}><p>{person(item)} · {money(item.amount_minor)}</p><p>{statusName(item.status)}</p><p className="mt-1 break-all text-ink-secondary">{item.id}</p>{item.payout_id&&<p>{t('payoutRecord')}: {item.payout_id} · {item.payout_occurred_at ? new Date(item.payout_occurred_at).toLocaleString() : t('legacyTime')}{item.payout_reference&&` · ${item.payout_reference}`}</p>}{item.reversed_minor ? <p>{t('reversed')}: {money(item.reversed_minor)} · {t('remaining')}: {money(item.recovery_pending_minor||0)}</p>:null}{(settlements||(item as Settlement&{settlement_id?:string}).settlement_id)&&<Button variant="outline" className="mt-3" onClick={e=>{trigger.current=e.currentTarget;update({settlement_id:settlements?item.id:(item as Settlement&{settlement_id:string}).settlement_id});}}>{t('detail')}</Button>}</li>)}</ul></ListResourceView>
    {list.page.total!==undefined&&<p className="text-sm">{t('total',{count:list.page.total})}</p>}<div className="flex gap-2">{params.get('cursor')&&<Button variant="outline" onClick={()=>update({cursor:''})}>{t('firstPage')}</Button>}{list.page.next_cursor&&<Button variant="outline" onClick={()=>update({cursor:list.page.next_cursor!})}>{t('nextPage')}</Button>}</div>
    <Dialog open={Boolean(selectedID)} onOpenChange={open=>{if(!open)update({settlement_id:''});}}><DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto" onCloseAutoFocus={e=>{if(trigger.current){e.preventDefault();trigger.current.focus();}}}><DialogTitle>{t('detail')}</DialogTitle><DialogDescription>{detail?`${person(detail.item)} · ${money(detail.item.amount_minor)}`:t('loading')}</DialogDescription>{error?<div role="alert"><p>{error}</p><Button variant="outline" onClick={()=>setRetry(v=>v+1)}>{t('retry')}</Button></div>:!detail?<p role="status">{t('loading')}</p>:<div className="space-y-3 text-sm"><p className="break-all">{detail.item.id} · {statusName(detail.item.status)}</p><p>{t('originalAmount')}: {money(detail.item.amount_minor)} · {t('reversed')}: {money(detail.item.reversed_minor||0)} · {t('remaining')}: {money(detail.item.recovery_pending_minor||0)}</p>{detail.item.payout_id&&<p className="break-all">{t('payoutRecord')}: {detail.item.payout_id} · {t('occurredAt')}: {detail.item.payout_occurred_at?new Date(detail.item.payout_occurred_at).toLocaleString():t('legacyTime')} · {t('recordedAt')}: {detail.item.payout_recorded_at?new Date(detail.item.payout_recorded_at).toLocaleString():'—'}{detail.item.payout_reference&&` · ${detail.item.payout_reference}`}{detail.item.payout_note&&` · ${detail.item.payout_note}`}</p>}{!detail.item.entries_snapshot_complete&&<p>{t('legacyEntries')}</p>}<ul className="space-y-2">{detail.entries.map(entry=><li className="break-all border-t border-hairline pt-2" key={entry.id}>{entry.id} · {money(entry.amount_minor)} · {statusName(entry.status)}<p>{entry.usage_event_id} · {entry.policy_version}</p></li>)}</ul></div>}</DialogContent></Dialog>
  </section>;
}
export default function ChannelSettlements() { return <ChannelCommissionRecords settlements />; }
