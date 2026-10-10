"use client";
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {ListResourceView} from '@/components/console/list-resource-view';
import {useListResource} from '@/hooks/use-list-resource';
import {apiBase} from '@/lib/api';
import {formatUsdMinor} from '@/lib/money';
type PnL={channel_org_id:string;business_type:string;pool_status:string;recharge_minor:number;unconsumed_minor:number;consumed_minor:number;marketing_minor:number;supplier_minor:number;pnl_minor:number;sell_minor:number;model_cost_minor:number;margin_minor:number;purchase_minor:number;self_revenue_minor:number;oem_sales_minor:number};
export function BrandPnLPanel({scope,ownerID,path}:{scope:string;ownerID:string;path:string}){
 const t=useTranslations('commissionWorkflow'),tl=useTranslations('ledgerWorkflow');
 const pnl=useListResource<PnL>({queryKey:`${scope}:${path}`,enabled:Boolean(scope),load:async()=>{try{const r=await fetch(`${apiBase}${path}`,{credentials:'include'});const b=await r.json();const valid=b.pnl?.channel_org_id===ownerID&&['sell_minor','model_cost_minor','margin_minor','pnl_minor','marketing_minor','supplier_minor','recharge_minor','unconsumed_minor','purchase_minor'].every(key=>Number.isFinite(b.pnl[key]));return {ok:r.ok&&valid,status:r.status,items:valid?[b.pnl]:[],message:b.error?.message};}catch{return{ok:false,network:true};}}});
 const current=pnl.snapshot.items[0];
 const platform=current?.business_type==='platform';
 function metrics(keys:(keyof PnL)[]){return <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">{keys.map(key=><div key={key}><dt className="text-sm text-ink-secondary">{tl(`metric.${key}`)}</dt><dd className="mt-1 font-mono text-lg">{['recharge_minor','unconsumed_minor'].includes(key)&&current?.pool_status!=='available'?tl('poolMissing'):`${formatUsdMinor(current?.[key] as number|undefined)} USD`}</dd></div>)}</dl>}
 return <section className="space-y-4 rounded-card border border-hairline bg-canvas-raised p-5"><div className="flex flex-wrap justify-between gap-3"><h2 className="text-lg font-semibold">{tl('tab.overview')}</h2><Button variant="outline" disabled={pnl.refreshing} onClick={()=>void pnl.reload()}>{t('refresh')}</Button></div><p className="text-sm text-ink-secondary">{tl(platform?'platformBasis':'basis')}</p><ListResourceView snapshot={pnl.snapshot} emptyTitle={t('empty')} emptyDetail={t('emptyDetail')} onRetry={()=>void pnl.reload()}>{current&&<div className="space-y-6">{metrics(platform?['self_revenue_minor','oem_sales_minor','sell_minor','model_cost_minor','margin_minor','marketing_minor','pnl_minor']:['sell_minor','model_cost_minor','margin_minor','marketing_minor','pnl_minor'])}<section className="space-y-3 border-t border-hairline pt-4"><h3 className="font-medium">{tl('cashAndStock')}</h3><p className="text-sm text-ink-secondary">{tl('cashBasis')}</p>{metrics(platform?['supplier_minor']:['purchase_minor','supplier_minor','recharge_minor','unconsumed_minor'])}</section></div>}</ListResourceView></section>;
}
