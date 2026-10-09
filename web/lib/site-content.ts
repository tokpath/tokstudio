
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

/** External snapshots are not evidence of this brand’s capabilities or usage. */
export async function loadSite(_host: string): Promise<SiteContent> {
  return {};
}
