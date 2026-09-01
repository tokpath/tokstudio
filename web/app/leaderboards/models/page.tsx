import { headers } from "next/headers";
import { PublicSection } from "@/components/public-section";
import { LeaderboardTable } from "@/components/leaderboard-table";
import { loadSite } from "@/lib/site-content";
import { getTranslations } from "next-intl/server";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function ModelsLeaderboardPage() {
  const t = await getTranslations("boardsUi");
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const site = await loadSite(host);
  const rows = site.leaderboards?.models || [];

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-10 px-6 py-20">
      <I18nPublicHero id="leaderboardsModels" primaryHref="/models" secondaryHref="/leaderboards/apps" />
      <PublicSection eyebrow="SHARE" title={t("modelsTitle")}>
        <LeaderboardTable rows={rows} />
      </PublicSection>
      <p className="text-sm text-ink-mute">{site.leaderboards?.note}</p>
    </main>
  );
}
