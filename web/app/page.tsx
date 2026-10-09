import Link from "next/link";
import { headers } from "next/headers";
import { getTranslations } from "next-intl/server";
import { Button } from "@/components/ui/button";
import { PublicMain, PublicPageHero, PublicSection } from "@/components/public-section";
import { Card, CardTitle } from "@/components/ui/card";
import { loadCatalogPage, priceForModel } from "@/lib/catalog";
import { loginIntentHref } from "@/lib/auth-intent";
import { loadPublicPlans } from "@/lib/public-plans";
import PublicStorefront from "./storefront";

export default async function PublicHome({searchParams}: {searchParams: Promise<{promotion_code?:string; promo?:string}>}) {
  const requestHeaders = await headers();
  const host = requestHeaders.get("x-tokenhub-host") || requestHeaders.get("host") || "localhost";
  const search = await searchParams;
  const promo = search.promotion_code || search.promo;
  const t = await getTranslations("publicExperience");
  const catalog = await getTranslations("catalog");
  const [models, plans] = await Promise.all([loadCatalogPage(host,{limit:6}),loadPublicPlans(host)]);
  return <PublicMain>
    <PublicPageHero eyebrow="API" title={t("title")} description={t("lead")} primaryHref={loginIntentHref("/app/keys",promo)} primaryLabel={t("createKey")} secondaryHref="/models" secondaryLabel={t("browseModels")} />
    <PublicSection title={t("modelsTitle")} description={t("modelsDetail")} action={<Button asChild variant="outline"><Link href="/models">{t("allModels")}</Link></Button>}>
      {!models.ok ? <Card><p role="alert">{t("modelsError")}</p><Link className="mt-3 inline-block text-brand-emphasis underline" href="/models">{t("retryModels")}</Link></Card> : !models.items.length ? <Card><p role="status">{t("modelsEmpty")}</p></Card> : <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">{models.items.map(model => <Card key={model.id}><CardTitle><Link href={`/models/${model.id.split("/").map(encodeURIComponent).join("/")}`} className="text-ink hover:text-brand-emphasis">{model.display_name}</Link></CardTitle><p className="my-3 break-all font-mono text-xs text-ink-mute">{model.id}</p><p className="font-mono text-sm">{priceForModel(model,{perSec:catalog("perSec"),perImage:catalog("perImage")}).primary}</p><Link className="mt-4 inline-block text-sm text-brand-emphasis underline" href={`/models/${model.id.split("/").map(encodeURIComponent).join("/")}`}>{t("viewModel")}</Link></Card>)}</div>}
    </PublicSection>
    <div className="grid gap-4 md:grid-cols-3">{["key", "docs", "referral"].map(key => <Card key={key}><CardTitle>{t(`${key}Title`)}</CardTitle><p className="my-3 text-sm text-ink-secondary">{t(`${key}Detail`)}</p><Link className="text-sm text-brand-emphasis underline" href={key === "docs" ? "/docs" : loginIntentHref(key === "key" ? "/app/keys" : "/app/referral",promo)}>{t(`${key}Action`)}</Link></Card>)}</div>
    <PublicStorefront models={[]} plans={plans.items} plansLoaded={plans.ok} promotionCode={promo} />
    <div className="flex flex-wrap gap-5 text-sm"><Link className="text-brand-emphasis underline" href="/enterprise">{t("oem")}</Link><Link className="text-ink-secondary underline" href="/trust">{t("trust")}</Link><Link className="text-ink-secondary underline" href="/terms">{t("terms")}</Link><Link className="text-ink-secondary underline" href="/privacy">{t("privacy")}</Link></div>
  </PublicMain>;
}
