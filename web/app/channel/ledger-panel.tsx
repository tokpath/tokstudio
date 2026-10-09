"use client";
import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useTranslations } from 'next-intl';
import { useViewer } from '@/components/rbac/viewer-context';
import { canChannelAction } from '@/lib/rbac';
import { AdminSupplierPanel } from '@/app/admin/billing/supplier-panel';
import { person, useCommissionPage, type Context, type Person } from '@/app/admin/commission/workflow-client';
import { BrandPnLPanel } from './brand-pnl-panel';
import { ListResourceView } from '@/components/console/list-resource-view';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { apiBase } from '@/lib/api';
import { formatUsdMinor } from '@/lib/money';

type Allocation={id:string;recipient?:Person;user_id:string;granted_minor:number;consumed_minor:number;remaining_minor:number;status:string;source_type:string;source_id:string;created_at:string};
export default function ChannelLedger(){
 const viewer=useViewer(),t=useTranslations('commissionWorkflow');const [context,setContext]=useState<Context|null>(null),[error,setError]=useState(''),[retry,setRetry]=useState(0);
 useEffect(()=>{setContext(null);setError('');if(!viewer.userId)return;let active=true;void fetch(`${apiBase}/channel/commission-context`,{credentials:'include'}).then(async r=>{const b=await r.json();if(!r.ok||!b.owner_id)throw new Error(b.error?.message||t('loadFailed'));if(active)setContext(b);}).catch(e=>{if(active)setError(e instanceof Error?e.message:t('loadFailed'));});return()=>{active=false;};},[viewer.userId,retry,t]);
 if(!context)return <section className="rounded-card border border-hairline p-5"><p role={error?'alert':'status'}>{error||t('loadingScope')}</p>{error&&<Button variant="outline" onClick={()=>setRetry(v=>v+1)}>{t('retry')}</Button>}</section>;
 const scope=`${viewer.userId}:${typeof window==='undefined'?'':window.location.host}:${context.owner_id}`;
 return <LedgerFacts key={scope} context={context} scope={scope} writable={canChannelAction('finance',viewer)}/>;
}
function LedgerFacts({context,scope,writable}:{context:Context;scope:string;writable:boolean}){
 const t=useTranslations('commissionWorkflow'),tl=useTranslations('ledgerWorkflow');const [params,setParams]=useState(new URLSearchParams()),[search,setSearch]=useState('');
 useEffect(()=>{const read=()=>{const p=new URLSearchParams(window.location.search);setParams(p);setSearch(p.get('allocation_q')||'');};read();window.addEventListener('popstate',read);return()=>window.removeEventListener('popstate',read);},[]);
 function update(values:Record<string,string>){const p=new URLSearchParams(window.location.search);Object.entries(values).forEach(([key,v])=>v?p.set(key,v):p.delete(key));window.history.pushState(null,'',`${window.location.pathname}?${p}`);setParams(p);}
 const tab=params.get('tab')||'overview';const q=new URLSearchParams({limit:'30'});if(params.get('channel_id'))q.set('channel_id',params.get('channel_id')!);if(params.get('allocation_q'))q.set('q',params.get('allocation_q')!);if(params.get('allocation_status'))q.set('status',params.get('allocation_status')!);if(params.get('allocation_cursor'))q.set('cursor',params.get('allocation_cursor')!);
 const allocations=useCommissionPage<Allocation>(scope,`/channel/allocations?${q}`);
 const ready=['ready','empty'].includes(allocations.snapshot.phase)&&!allocations.refreshing;
 const returnURL=`/channel/ledger?${params}`;
 return <div className="space-y-5"><div className="flex flex-wrap gap-2" role="navigation" aria-label={tl('title')}>{['overview','issuance','supplier'].map(key=><Button variant={tab===key?'default':'outline'} aria-pressed={tab===key} key={key} onClick={()=>update({tab:key})}>{tl(`tab.${key}`)}</Button>)}</div><p className="text-sm">{t('brand')}: {context.owner_name||context.owner_code||context.owner_id}</p>
 {tab==='supplier'?<AdminSupplierPanel prefix="/channel" context={context}/>:tab==='issuance'?<section className="space-y-4 rounded-card border border-hairline bg-canvas-raised p-5"><div className="flex flex-wrap justify-between gap-3"><h2 className="text-lg font-semibold">{tl('tab.issuance')}</h2>{writable&&<Link className="text-sm text-brand-emphasis underline" href={`/channel/payments/orders?from=ledger&next=${encodeURIComponent(returnURL)}`}>{tl('goPayments')}</Link>}</div><form className="flex flex-wrap gap-2" onSubmit={e=>{e.preventDefault();update({allocation_q:search.trim(),allocation_cursor:''});}}><Input aria-label={t('search')} placeholder={t('searchHint')} value={search} onChange={e=>setSearch(e.target.value)}/><Button variant="outline">{t('search')}</Button><Button type="button" variant="outline" onClick={()=>void allocations.reload()}>{t('refresh')}</Button></form><ListResourceView snapshot={allocations.snapshot} emptyTitle={t('empty')} emptyDetail={t('emptyDetail')} onRetry={()=>void allocations.reload()}><ul className="grid gap-3">{allocations.snapshot.items.map(item=><li key={item.id} className="rounded-control border border-hairline p-4 text-sm"><p className="break-all font-medium">{person(item)}</p><p>{tl('granted')}: {formatUsdMinor(item.granted_minor)} USD · {tl('used')}: {formatUsdMinor(item.consumed_minor)} USD · {tl('remaining')}: {formatUsdMinor(item.remaining_minor)} USD</p><p>{tl.has(`status.${item.status}`)?tl(`status.${item.status}`):item.status} · {new Date(item.created_at).toLocaleString()}</p><p className="mt-1 break-all text-ink-secondary">{item.id} · {item.source_type} · {item.source_id}</p></li>)}</ul></ListResourceView>{allocations.page.total!==undefined&&<div className="flex flex-wrap items-center gap-3 text-sm"><span>{t('total',{count:allocations.page.total})}</span>{params.get('allocation_cursor')&&<Button variant="outline" disabled={!ready} onClick={()=>update({allocation_cursor:''})}>{t('firstPage')}</Button>}{allocations.page.next_cursor&&<Button variant="outline" disabled={!ready} onClick={()=>update({allocation_cursor:allocations.page.next_cursor!})}>{t('nextPage')}</Button>}</div>}</section>:<BrandPnLPanel scope={scope} ownerID={context.owner_id} path="/channel/pnl"/>}
 </div>;
}
