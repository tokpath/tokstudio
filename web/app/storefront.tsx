"use client";

import Link from "next/link";
import { Card, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { formatUsdMinor } from "@/lib/money";
import { useTranslations } from "next-intl";

type PublicModel = { id?: string; display_name?: string; vendor?: string };
type PublicPlan = { id?: string; name?: string; price_minor?: number; currency?: string; billing_period?: string; auto_renew_allowed?: boolean };
export default function PublicStorefront({ models, plans }: { models: PublicModel[]; plans: PublicPlan[] }) {
  const t = useTranslations('storefront');
  const period: Record<string, string> = { once: '一次购买', one_time: '一次购买', month: '每月', monthly: '每月', quarter: '每季度', quarterly: '每季度', year: '每年', yearly: '每年' };
  return <div className="space-y-16">
    <section id="models"><h2 className="mb-6 text-2xl font-semibold">{t('modelsTitle')}</h2><div className="grid gap-4 md:grid-cols-3">{models.map(model => <Card key={model.id}><CardTitle>{model.display_name || model.id}</CardTitle><p className="my-3 break-all font-mono text-sm text-ink-mute">{model.id}</p><Link className="text-brand-emphasis underline" href={`/models/${model.id?.split('/').map(encodeURIComponent).join('/')}`}>查看价格与使用说明</Link></Card>)}</div>{!models.length ? <p role="status">当前品牌暂无可用模型。</p> : null}</section>
    {plans.length ? <section id="plans"><h2 className="mb-6 text-2xl font-semibold">{t('plansTitle')}</h2><div className="grid gap-4 md:grid-cols-2">{plans.map(plan => <Card key={plan.id}><CardTitle>{plan.name}</CardTitle><p className="my-4 font-mono text-2xl tabular-nums">{plan.currency === 'USD' || !plan.currency ? formatUsdMinor(plan.price_minor) : `${plan.price_minor === undefined ? '—' : plan.price_minor / 1_000_000} ${plan.currency}`}</p><p className="text-sm text-ink-secondary">{period[plan.billing_period || ''] || '购买周期待确认'} · {plan.auto_renew_allowed ? '可选择自动续费' : '不自动续费'}</p><Button asChild className="mt-4"><Link href={`/app/plans?plan=${encodeURIComponent(plan.id || '')}`}>查看权益与购买方式</Link></Button></Card>)}</div></section> : null}
    <Card id="topup"><CardTitle>充值与套餐</CardTitle><p className="my-4 text-sm text-ink-secondary">登录后查看所属品牌的商品、额度与支付方式，确认正式报价后购买。</p><Button asChild><Link href="/app/wallet">进入收银台</Link></Button></Card>
  </div>;
}
