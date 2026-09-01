import { getTranslations } from "next-intl/server";
import { PublicSection, PublicMain } from "@/components/public-section";
import { IconStamp } from "@/components/icon-stamp";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { VERIFY_ICONS } from "@/lib/page-icons";

export default async function VerifyPage() {
  const t = await getTranslations("verifyUi");
  return (
    <PublicMain>
      <I18nPublicHero id="verify" primaryHref="/app" secondaryHref="/docs" />
      <PublicSection eyebrow="METHOD" title={t("howTitle")}>
        <ul className="space-y-3 text-sm text-ink-secondary">
          {[0, 1].map((i) => (
            <li key={i} className="flex items-start gap-3 rounded-card border border-hairline bg-canvas-raised px-4 py-3">
              <IconStamp icon={VERIFY_ICONS[i]} size="sm" className="mt-0.5" />
              <span>{t(`l${i}`)}</span>
            </li>
          ))}
        </ul>
      </PublicSection>
    </PublicMain>
  );
}
