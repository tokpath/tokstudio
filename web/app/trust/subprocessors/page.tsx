import { getTranslations } from "next-intl/server";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { PublicMain, PublicSection } from "@/components/public-section";

export default async function SubprocessorsPage() {
  const t = await getTranslations("trustUi");
  return (
    <PublicMain>
      <I18nPublicHero id="trustSubprocessors" primaryHref="/trust" />
      <PublicSection title={t("supplierPendingTitle")}>
        <p className="max-w-2xl text-sm leading-relaxed text-ink-secondary">{t("supplierPendingDetail")}</p>
      </PublicSection>
    </PublicMain>
  );
}
