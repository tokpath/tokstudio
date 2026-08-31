import { headers } from "next/headers";
import { PublicSection } from "@/components/public-section";
import { LeaderboardTable } from "@/components/leaderboard-table";
import { loadSite } from "@/lib/site-content";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function AppsLeaderboardPage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const site = await loadSite(host);
  const rows = site.leaderboards?.apps || [];

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-10 px-6 py-20">
      <I18nPublicHero id="leaderboardsApps" primaryHref="/vibe-coding" secondaryHref="/leaderboards/models" />
      <PublicSection eyebrow="TOOLS" title="大家都在用什么客户端？">
        <LeaderboardTable rows={rows} hrefOf={() => "/vibe-coding"} />
      </PublicSection>
    </main>
  );
}
