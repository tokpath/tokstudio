import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { Button } from "@/components/ui/button";
import { PublicSection } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function PromoPage() {
  const t = await getTranslations("promoUi");
  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-12 px-6 py-20">
      <I18nPublicHero id="promo" primaryHref="/login" secondaryHref="/channel" />
      <PublicSection eyebrow="HOW" title={t("howTitle")}>
        <ol className="list-decimal space-y-3 pl-5 text-sm text-ink-secondary">
          <li>{t("l0")}</li>
          <li>{t("l1")}</li>
          <li>{t("l2")}</li>
        </ol>
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
