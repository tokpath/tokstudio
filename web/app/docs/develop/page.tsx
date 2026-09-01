import { getTranslations } from "next-intl/server";
import { PublicSection } from "@/components/public-section";
import { IconStamp } from "@/components/icon-stamp";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { DOCS_ERROR_ICONS } from "@/lib/page-icons";

export default async function DocsDevelopPage() {
  const t = await getTranslations("docsUi");
  return (
    <main className="mx-auto flex w-full max-w-[720px] flex-col gap-10 px-6 py-20">
      <I18nPublicHero id="docsDevelop" primaryHref="/docs" secondaryHref="/trust" />
      <PublicSection eyebrow="ERRORS" title={t("errTitle")}>
        <ul className="space-y-3 text-sm text-ink-secondary">
          {[
            { code: "402", key: "e402" as const, tone: "text-danger" },
            { code: "429", key: "e429" as const, tone: "text-hold" },
            { code: "403", key: "e403" as const, tone: "text-danger" },
          ].map((item, i) => (
            <li key={item.code} className="flex items-start gap-3 rounded-card border border-hairline bg-canvas-raised px-4 py-3">
              <IconStamp icon={DOCS_ERROR_ICONS[i]} size="sm" className="mt-0.5" />
              <span>
                <span className={`font-mono ${item.tone}`}>{item.code}</span> {t(item.key)}
              </span>
            </li>
          ))}
        </ul>
      </PublicSection>
      <PublicSection eyebrow="SECURITY" title={t("secTitle")}>
        <p className="text-sm text-ink-secondary">{t("secBody")}</p>
      </PublicSection>
    </main>
  );
}
