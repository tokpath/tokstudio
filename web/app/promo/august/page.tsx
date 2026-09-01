import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { Button } from "@/components/ui/button";
import { PublicSection, PublicMain } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function AugustPromoPage() {
  const t = await getTranslations("promoUi");
  return (
    <PublicMain>
      <I18nPublicHero id="promoAugust" primaryHref="/login" secondaryHref="/app" />
      <PublicSection eyebrow="CODE" title={t("howCode")}>
        <div className="rounded-card border border-hairline bg-canvas-raised p-5">
          <p className="font-mono text-lg">THE2E</p>
          <p className="mt-2 text-sm text-ink-secondary">{t("codeLead")}</p>
        </div>
      </PublicSection>
      <Button asChild variant="outline" className="w-fit">
        <Link href="/">{t("backHome")}</Link>
      </Button>
    </PublicMain>
  );
}
