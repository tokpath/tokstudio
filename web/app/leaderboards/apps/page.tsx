import { headers } from "next/headers";
import { PublicPageHero, PublicSection } from "@/components/public-section";
import { LeaderboardTable } from "@/components/leaderboard-table";
import { loadSite } from "@/lib/site-content";

export default async function AppsLeaderboardPage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const site = await loadSite(host);
  const rows = site.leaderboards?.apps || [];

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-10 px-6 py-20">
      <PublicPageHero
        eyebrow="LEADERBOARD · APPS"
        title="应用 / 工具用量排行"
        description="对齐 ofox 应用榜。SDK 与 CLI 份额来自公开快照。"
        primaryHref="/vibe-coding"
        primaryLabel="编程工具接入"
        secondaryHref="/leaderboards/models"
        secondaryLabel="模型榜"
      />
      <PublicSection eyebrow="TOOLS" title="大家都在用什么客户端？">
        <LeaderboardTable rows={rows} hrefOf={() => "/vibe-coding"} />
      </PublicSection>
    </main>
  );
}
