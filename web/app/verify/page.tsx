import { getTranslations } from "next-intl/server";
import { PublicSection } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function VerifyPage() {
  const t = await getTranslations("verifyUi");
  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-12 px-6 py-20">
      <I18nPublicHero id="verify" primaryHref="/app" secondaryHref="/docs" />
      <PublicSection eyebrow="METHOD" title={t("howTitle")}>
        <ul className="space-y-3 text-sm text-ink-secondary">
          <li className="rounded-card border border-hairline bg-canvas-raised px-4 py-3">{t("l0")}</li>
          <li className="rounded-card border border-hairline bg-canvas-raised px-4 py-3">{t("l1")}</li>
        </ul>
      </PublicSection>
    </main>
  );
}
