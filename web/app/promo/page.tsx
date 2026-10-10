import { getTranslations } from "next-intl/server";
import { PublicMain, PublicPageHero } from "@/components/public-section";
import { loginIntentHref } from "@/lib/auth-intent";

export default async function PromoPage({searchParams}: {searchParams: Promise<{promotion_code?:string; promo?:string}>}) {
  const search = await searchParams;
  const t = await getTranslations("publicExperience");
  return <PublicMain width="prose"><PublicPageHero eyebrow="API" title={t("referralTitle")} description={t("referralDetail")} primaryHref={loginIntentHref("/app/referral",search.promotion_code || search.promo)} primaryLabel={t("referralAction")} secondaryHref="/models" secondaryLabel={t("browseModels")} /></PublicMain>;
}
