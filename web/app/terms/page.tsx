import { getTranslations } from "next-intl/server";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { PublicMain } from "@/components/public-section";

export default async function TermsPage() {
  const t = await getTranslations("legalUi");
  return (
    <PublicMain width="prose">
      <I18nPublicHero id="terms" primaryHref="/privacy" />
      <article className="space-y-4 text-sm leading-relaxed text-ink-secondary">
        <p>{t("t0")}</p>
        <p>{t("t1")}</p>
        <p>{t("t2")}</p>
      </article>
    </PublicMain>
  );
}
