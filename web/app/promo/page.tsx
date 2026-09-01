import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { Button } from "@/components/ui/button";
import { PublicSection } from "@/components/public-section";
import { FeatureCard } from "@/components/feature-card";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { PROMO_STEP_ICONS } from "@/lib/page-icons";

export default async function PromoPage() {
  const t = await getTranslations("promoUi");
  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-12 px-6 py-20">
      <I18nPublicHero id="promo" primaryHref="/login" secondaryHref="/channel" />
      <PublicSection eyebrow="HOW" title={t("howTitle")}>
        <div className="grid gap-3 md:grid-cols-3">
          {[0, 1, 2].map((i) => (
            <FeatureCard key={i} icon={PROMO_STEP_ICONS[i]} title={t(`l${i}`)} />
          ))}
        </div>
      </PublicSection>
      <div className="flex flex-wrap gap-3">
        <Button asChild variant="outline">
          <Link href="/partner">{t("partner")}</Link>
        </Button>
        <Button asChild variant="outline">
          <Link href="/docs">{t("docs")}</Link>
        </Button>
      </div>
    </main>
  );
}
