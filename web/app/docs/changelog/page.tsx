import { getTranslations } from "next-intl/server";
import { PublicSection, PublicMain } from "@/components/public-section";
import { FeatureCard } from "@/components/feature-card";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { CHANGELOG_ICONS } from "@/lib/page-icons";

export default async function ChangelogPage() {
  const t = await getTranslations("docsUi");
  const items = [0, 1, 2].map((i) => ({
    date: "2026-08",
    title: t(`c${i}t`),
    detail: t(`c${i}d`),
    icon: CHANGELOG_ICONS[i],
  }));

  return (
    <PublicMain width="prose">
      <I18nPublicHero id="docsChangelog" primaryHref="/docs" />
      <PublicSection eyebrow="LOG" title={t("logTitle")}>
        <ol className="grid gap-4">
          {items.map((item) => (
            <li key={item.title}>
              <FeatureCard icon={item.icon} title={item.title} description={item.detail} meta={item.date} />
            </li>
          ))}
        </ol>
      </PublicSection>
    </PublicMain>
  );
}
