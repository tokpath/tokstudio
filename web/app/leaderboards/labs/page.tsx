import { headers } from "next/headers";
import { PublicSection, PublicMain } from "@/components/public-section";
import { LeaderboardTable } from "@/components/leaderboard-table";
import { loadSite } from "@/lib/site-content";
import { getTranslations } from "next-intl/server";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function LabsLeaderboardPage() {
  const t = await getTranslations("boardsUi");
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const site = await loadSite(host);
  const rows = site.leaderboards?.labs || [];

  return (
    <PublicMain>
      <I18nPublicHero id="leaderboardsLabs" primaryHref="/models" secondaryHref="/leaderboards/models" />
      <PublicSection eyebrow="VENDORS" title={t("labsTitle")}>
        <LeaderboardTable rows={rows} hrefOf={(row) => `/models?q=${encodeURIComponent(row.name)}`} />
      </PublicSection>
    </PublicMain>
  );
}
