import { headers } from "next/headers";
import { getTranslations } from "next-intl/server";
import { PublicMain, PublicPageHero } from "@/components/public-section";
import { loadPublicPlans } from "@/lib/public-plans";
import { loginIntentHref } from "@/lib/auth-intent";
import PublicStorefront from "../storefront";

export default async function PricingPage({searchParams}: {searchParams: Promise<{promotion_code?: string; promo?: string}>}) {
  const h = await headers();
  const host = h.get("x-tokenhub-host") || h.get("host") || "localhost";
  const search = await searchParams;
  const promo = search.promotion_code || search.promo;
  const plans = await loadPublicPlans(host);
  const t = await getTranslations("publicExperience");
  return <PublicMain><PublicPageHero eyebrow="API" title={t("pricingTitle")} description={t("pricingDetail")} primaryHref="/models" primaryLabel={t("browseModels")} secondaryHref={loginIntentHref("/app/wallet",promo)} secondaryLabel={t("walletAction")} /><PublicStorefront models={[]} plans={plans.items} plansLoaded={plans.ok} promotionCode={promo} /></PublicMain>;
}
