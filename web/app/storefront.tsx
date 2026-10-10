"use client";

import Link from "next/link";
import { Card, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { useTranslations } from "next-intl";
import { loginIntentHref } from "@/lib/auth-intent";
import { planPeriod, planPrice, type PublicPlan } from "@/lib/public-plans";

type PublicModel = { id?: string; display_name?: string; vendor?: string };
export default function PublicStorefront({ models, plans, plansLoaded = true, promotionCode }: { models: PublicModel[]; plans: PublicPlan[]; plansLoaded?: boolean; promotionCode?: string }) {
  const t = useTranslations("publicExperience");
  return <div className="space-y-8">
    {models.length > 0 && <section id="models"><h2 className="mb-6 text-2xl font-semibold">{t("modelsTitle")}</h2><div className="grid gap-4 md:grid-cols-3">{models.map(model => <Card key={model.id}><CardTitle>{model.display_name || model.id}</CardTitle><p className="my-3 break-all font-mono text-sm text-ink-mute">{model.id}</p><Link className="text-brand-emphasis underline" href={`/models/${model.id?.split('/').map(encodeURIComponent).join('/')}`}>{t("viewModel")}</Link></Card>)}</div></section>}
    <section id="plans"><h2 className="mb-5 text-2xl font-semibold">{t("plansTitle")}</h2>
      {!plansLoaded ? <Card><p role="alert">{t("plansError")}</p></Card> : plans.length ? <div className="grid gap-4 md:grid-cols-2">{plans.map(plan => <Card key={plan.id}><CardTitle>{plan.name}</CardTitle><p className="my-4 font-mono text-2xl tabular-nums">{planPrice(plan)}</p><p className="text-sm text-ink-secondary">{t(`period_${planPeriod(plan.billing_period)}`)} · {t("renewManual")}</p><Button asChild className="mt-4"><Link href={loginIntentHref(`/app/plans?plan=${encodeURIComponent(plan.id)}`,promotionCode)}>{t("buyPlan")}</Link></Button></Card>)}</div> : <p className="text-sm text-ink-secondary">{t("plansEmpty")}</p>}
    </section>
    <Card id="topup"><CardTitle>{t("walletTitle")}</CardTitle><p className="my-4 text-sm text-ink-secondary">{t("walletDetail")}</p><Button asChild><Link href={loginIntentHref("/app/wallet",promotionCode)}>{t("walletAction")}</Link></Button></Card>
  </div>;
}
