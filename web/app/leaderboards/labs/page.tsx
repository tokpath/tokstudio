import { headers } from "next/headers";
import { PublicPageHero, PublicSection } from "@/components/public-section";
import { LeaderboardTable } from "@/components/leaderboard-table";
import { loadSite } from "@/lib/site-content";

export default async function LabsLeaderboardPage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const site = await loadSite(host);
  const rows = site.leaderboards?.labs || [];

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-10 px-6 py-20">
      <PublicPageHero
        eyebrow="LEADERBOARD · LABS"
        title="实验室 / 厂商用量排行"
        description="对齐 ofox labs 榜：按供应商聚合的 token 份额。"
        primaryHref="/models"
        primaryLabel="按厂商看价目"
        secondaryHref="/leaderboards/models"
        secondaryLabel="模型榜"
      />
      <PublicSection eyebrow="VENDORS" title="厂商份额">
        <LeaderboardTable rows={rows} hrefOf={(row) => `/models?q=${encodeURIComponent(row.name)}`} />
      </PublicSection>
    </main>
  );
}
