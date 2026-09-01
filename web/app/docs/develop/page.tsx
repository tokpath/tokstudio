import { getTranslations } from "next-intl/server";
import { PublicSection } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function DocsDevelopPage() {
  const t = await getTranslations("docsUi");
  return (
    <main className="mx-auto flex w-full max-w-[720px] flex-col gap-10 px-6 py-20">
      <I18nPublicHero id="docsDevelop" primaryHref="/docs" secondaryHref="/trust" />
      <PublicSection eyebrow="ERRORS" title={t("errTitle")}>
        <ul className="space-y-3 text-sm text-ink-secondary">
          <li className="rounded-card border border-hairline bg-canvas-raised px-4 py-3">
            <span className="font-mono text-danger">402</span> {t("e402")}
          </li>
          <li className="rounded-card border border-hairline bg-canvas-raised px-4 py-3">
            <span className="font-mono text-hold">429</span> {t("e429")}
          </li>
          <li className="rounded-card border border-hairline bg-canvas-raised px-4 py-3">
            <span className="font-mono text-danger">403</span> {t("e403")}
          </li>
        </ul>
      </PublicSection>
      <PublicSection eyebrow="SECURITY" title={t("secTitle")}>
        <p className="text-sm text-ink-secondary">{t("secBody")}</p>
      </PublicSection>
    </main>
  );
}
