"use client";
import { useEffect, useState } from 'react';
import { useTranslations } from 'next-intl';
import { useViewer } from '@/components/rbac/viewer-context';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { ListResourceView } from '@/components/console/list-resource-view';
import { useCommissionPage, person } from '@/app/admin/commission/workflow-client';
import type { Settlement } from '@/app/admin/commission/settlement-panel';
import { formatUsdMinor } from '@/lib/money';
export function ChannelCommissionRecords({ settlements = false }: { settlements?: boolean }) {
  const viewer = useViewer(), t = useTranslations('commissionWorkflow');
  const [params, setParams] = useState(new URLSearchParams()), [search, setSearch] = useState('');
  useEffect(() => { const read = () => { const p = new URLSearchParams(window.location.search); setParams(p); setSearch(p.get('q') || ''); }; read(); window.addEventListener('popstate',read); return () => window.removeEventListener('popstate',read); }, []);
  function update(values: Record<string,string>) { const p = new URLSearchParams(window.location.search); Object.entries(values).forEach(([key,v])=>v?p.set(key,v):p.delete(key)); window.history.pushState(null,'',`${window.location.pathname}?${p}`); setParams(p); }
  const q = new URLSearchParams(params); q.delete('tab'); q.delete('from'); q.delete('next'); q.set('limit','30');
  const scope = viewer.userId ? `${viewer.userId}:${typeof window === 'undefined' ? '' : window.location.host}:channel-self` : '';
  const list = useCommissionPage<Settlement>(scope, `/channel/${settlements ? 'settlements' : 'commissions'}?${q}`);
  return <section className="space-y-4 rounded-card border border-hairline bg-canvas-raised p-5"><h2 className="text-lg font-semibold">{t(settlements ? 'tab.payout' : 'tab.all')}</h2><form className="flex flex-wrap gap-2" onSubmit={e=>{e.preventDefault();update({q:search.trim(),cursor:''});}}><Input aria-label={t('search')} placeholder={t('searchHint')} value={search} onChange={e=>setSearch(e.target.value)}/><Button variant="outline">{t('search')}</Button><Button type="button" variant="outline" onClick={()=>void list.reload()}>{t('refresh')}</Button></form><ListResourceView snapshot={list.snapshot} emptyTitle={t('empty')} emptyDetail={t('emptyDetail')} onRetry={()=>void list.reload()}><ul className="grid gap-3">{list.snapshot.items.map(item=><li className="rounded-control border border-hairline p-4 text-sm" key={item.id}><p>{person(item)} · {formatUsdMinor(item.amount_minor)} USD</p><p>{t.has(`status.${item.status}`)?t(`status.${item.status}`):item.status}</p><p className="mt-1 break-all text-ink-secondary">{item.id}</p>{item.payout_id&&<p>{t('payoutRecord')}: {item.payout_id} · {item.payout_occurred_at ? new Date(item.payout_occurred_at).toLocaleString() : t('legacyTime')}{item.payout_reference&&` · ${item.payout_reference}`}</p>}{item.reversed_minor ? <p>{t('reversed')}: {formatUsdMinor(item.reversed_minor)} USD · {t('remaining')}: {formatUsdMinor(item.recovery_pending_minor)} USD</p>:null}</li>)}</ul></ListResourceView>{list.page.total!==undefined&&<p className="text-sm">{t('total',{count:list.page.total})}</p>}<div className="flex gap-2">{params.get('cursor')&&<Button variant="outline" onClick={()=>update({cursor:''})}>{t('firstPage')}</Button>}{list.page.next_cursor&&<Button variant="outline" onClick={()=>update({cursor:list.page.next_cursor!})}>{t('nextPage')}</Button>}</div></section>;
}
export default function ChannelSettlements() { return <ChannelCommissionRecords settlements />; }
