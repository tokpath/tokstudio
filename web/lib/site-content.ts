import fixture from "@/lib/fixtures/ofox-site.json";
import { fetchAPI } from "@/lib/api";

export type LeaderboardRow = {
  rank: string;
  vendor?: string;
  name: string;
  id?: string;
  share: string;
  delta: string;
};

export type SiteBlogPost = {
  title: string;
  href: string;
  source?: string;
  date?: string;
  summary?: string;
};

export type SiteApp = {
  slug: string;
  name: string;
  kind: string;
  oss?: boolean;
  summary: string;
  date?: string;
};

export type SiteContent = {
  source?: string;
  fetched?: string;
  leaderboards?: {
    models?: LeaderboardRow[];
    apps?: LeaderboardRow[];
    labs?: LeaderboardRow[];
    window?: string;
    note?: string;
  };
  apps?: SiteApp[];
  blog?: SiteBlogPost[];
  discounts?: Record<string, unknown>;
};

const FALLBACK = fixture as SiteContent;

/** 优先读服务端 dump 的 ofox 公开站内容，API 不可达时用快照。 */
export async function loadSite(host: string): Promise<SiteContent> {
  try {
    const data = await fetchAPI<{ site?: SiteContent }>("/v1/public/site", { host });
    if (data.site?.leaderboards || data.site?.blog) {
      return { ...FALLBACK, ...data.site };
    }
  } catch {
    /* fall through */
  }
  return FALLBACK;
}
