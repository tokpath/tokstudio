"use client";
import { useTranslations } from 'next-intl';
import { Button } from '@/components/ui/button';
import { ListResourceView } from '@/components/console/list-resource-view';
import { useListResource } from '@/hooks/use-list-resource';
import { apiBase } from '@/lib/api';
import { formatUsdMinor } from '@/lib/money';
type PnL={channel_org_id:string;pool_status:string;recharge_minor:number;unconsumed_minor:number;consumed_minor:number;marketing_minor:number;supplier_minor:number;pnl_minor:number};
export function BrandPnLPanel({scope,ownerID,path}:{scope:string;ownerID:string;path:string}){
 const t=useTranslations('commissionWorkflow'),tl=useTranslations('ledgerWorkflow');
 const pnl=useListResource<PnL>({queryKey:`${scope}:${path}`,enabled:Boolean(scope),load:async()=>{try{const r=await fetch(`${apiBase}${path}`,{credentials:'include'});const b=await r.json();const valid=b.pnl?.channel_org_id===ownerID&&['recharge_minor','unconsumed_minor','consumed_minor','marketing_minor','supplier_minor','pnl_minor'].every(key=>Number.isFinite(b.pnl[key]));return {ok:r.ok&&valid,status:r.status,items:valid?[b.pnl]:[],message:b.error?.message};}catch{return{ok:false,network:true};}}});
 const current=pnl.snapshot.items[0];
 return <section className="space-y-4 rounded-card border border-hairline bg-canvas-raised p-5"><div className="flex flex-wrap justify-between gap-3"><h2 className="text-lg font-semibold">{tl('tab.overview')}</h2><Button variant="outline" disabled={pnl.refreshing} onClick={()=>void pnl.reload()}>{t('refresh')}</Button></div><p className="text-sm text-ink-secondary">{tl('basis')}</p><ListResourceView snapshot={pnl.snapshot} emptyTitle={t('empty')} emptyDetail={t('emptyDetail')} onRetry={()=>void pnl.reload()}>{current&&<dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">{(['recharge_minor','unconsumed_minor','consumed_minor','marketing_minor','supplier_minor','pnl_minor'] as const).map(key=><div key={key}><dt className="text-sm text-ink-secondary">{tl(`metric.${key}`)}</dt><dd className="mt-1 font-mono text-lg">{['recharge_minor','unconsumed_minor'].includes(key)&&current.pool_status!=='available'?tl('poolMissing'):`${formatUsdMinor(current[key])} USD`}</dd></div>)}</dl>}</ListResourceView></section>;
}
