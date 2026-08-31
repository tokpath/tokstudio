import { getTranslations } from "next-intl/server";
import { PublicSection } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function ChangelogPage() {
  const t = await getTranslations("docsUi");
  const items = [0, 1, 2].map((i) => ({
    date: "2026-08",
    title: t(`c${i}t`),
    detail: t(`c${i}d`),
  }));

  return (
    <main className="mx-auto flex w-full max-w-[720px] flex-col gap-10 px-6 py-20">
      <I18nPublicHero id="docsChangelog" primaryHref="/docs" />
      <PublicSection eyebrow="LOG" title={t("logTitle")}>
        <ol className="space-y-3">
          {items.map((item) => (
            <li key={item.title} className="rounded-card border border-hairline bg-canvas-raised p-4">
              <p className="th-eyebrow text-ink-mute">{item.date}</p>
              <p className="mt-1 font-semibold">{item.title}</p>
              <p className="mt-1 text-sm text-ink-secondary">{item.detail}</p>
            </li>
          ))}
        </ol>
      </PublicSection>
    </main>
  );
}
