import { headers } from "next/headers";
import { PublicSection } from "@/components/public-section";
import { LeaderboardTable } from "@/components/leaderboard-table";
import { loadSite } from "@/lib/site-content";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function LabsLeaderboardPage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const site = await loadSite(host);
  const rows = site.leaderboards?.labs || [];

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-10 px-6 py-20">
      <I18nPublicHero id="leaderboardsLabs" primaryHref="/models" secondaryHref="/leaderboards/models" />
      <PublicSection eyebrow="VENDORS" title="厂商份额">
        <LeaderboardTable rows={rows} hrefOf={(row) => `/models?q=${encodeURIComponent(row.name)}`} />
      </PublicSection>
    </main>
  );
}
