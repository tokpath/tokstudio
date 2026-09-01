import { describe, expect, it } from "vitest";
import { PUBLIC_PAGE_SPECS } from "./public-site";

describe("ofox public page map", () => {
  it("covers public and authenticated main-flow pages", () => {
    const hrefs = PUBLIC_PAGE_SPECS.map((p) => p.href);
    for (const required of [
      "/",
      "/models",
      "/quickstart",
      "/docs",
      "/docs/integrations",
      "/enterprise",
      "/trust",
      "/best-value",
      "/model-finder",
      "/vibe-coding",
      "/video",
      "/image",
      "/leaderboards/models",
      "/leaderboards/apps",
      "/leaderboards/labs",
      "/awesome-ofox",
      "/compare",
      "/pricing",
      "/blog",
      "/login",
      "/app",
      "/app/playground",
      "/app/keys",
      "/app/catalog",
      "/app/usage",
      "/app/activity",
      "/app/wallet",
      "/app/referral",
      "/app/settings",
      "/app/settings/webhooks",
    ]) {
      expect(hrefs).toContain(required);
    }
    expect(PUBLIC_PAGE_SPECS.filter((p) => p.auth).map((p) => p.href)).toContain("/app");
    expect(PUBLIC_PAGE_SPECS.filter((p) => p.auth).map((p) => p.href)).toContain("/app/keys");
  });
});
