"use client";
import { useQuery } from "@tanstack/react-query";
import { AdminSupplierPanel } from "@/app/admin/billing/supplier-panel";
import { Card, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { apiClient } from "@/lib/client";
import { formatUsdMinor } from "@/lib/money";
import { useTranslations } from "next-intl";

type PnL = { recharge_minor?: number; unconsumed_minor?: number; consumed_minor?: number; marketing_minor?: number; supplier_minor?: number; pnl_minor?: number };
export default function ChannelLedger() {
  const t = useTranslations('channelUi');
  const query = useQuery({ queryKey: ['/channel/pnl'], queryFn: () => apiClient<{ pnl?: PnL; error?: { message?: string } }>('GET', '/channel/pnl') });
  const p = query.data?.pnl;
  const metrics = [{ k: t('pnlRecharge'), v: p?.recharge_minor }, { k: t('pnlUnconsumed'), v: p?.unconsumed_minor }, { k: t('pnlConsumed'), v: p?.consumed_minor }, { k: t('pnlMarketing'), v: p?.marketing_minor }, { k: t('pnlSupplier'), v: p?.supplier_minor }, { k: t('pnlResult'), v: p?.pnl_minor }];
  return <div className="space-y-6"><Card><CardTitle>经营账本</CardTitle>{query.isError || query.data?.error ? <p role="alert">经营汇总读取失败，请重试。</p> : null}<dl className="my-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">{metrics.map(item => <div key={item.k}><dt className="text-sm text-ink-secondary">{item.k}</dt><dd className="font-mono text-lg tabular-nums">{formatUsdMinor(item.v)}</dd></div>)}</dl><Button variant="outline" onClick={() => void query.refetch()}>刷新汇总</Button></Card><AdminSupplierPanel prefix="/channel" /></div>;
}
