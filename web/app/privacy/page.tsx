import { getTranslations } from "next-intl/server";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { PublicMain } from "@/components/public-section";

export default async function PrivacyPage() {
  const t = await getTranslations("legalUi");
  return (
    <PublicMain width="prose">
      <I18nPublicHero id="privacy" primaryHref="/trust" />
      <article className="space-y-4 text-sm leading-relaxed text-ink-secondary">
        <p>{t("p0")}</p>
        <p>{t("p1")}</p>
        <p>{t("p2")}</p>
      </article>
    </PublicMain>
  );
}
