import { describe, expect, it } from "vitest";
import { FOOTER_GROUPS, PUBLIC_PAGE_SPECS } from "./public-site";
import { MEGA_MENUS } from "./mega-nav";

describe("maintained public tasks", () => {
  it("keeps model and documentation access public and the API account together", () => {
    const pages=new Map(PUBLIC_PAGE_SPECS.map(page=>[page.href,page]));
    for(const path of ["/models","/docs","/docs/integrations","/pricing","/promo"]) expect(pages.get(path)?.auth).not.toBe(true);
    for(const path of ["/app","/app/keys","/app/wallet","/app/referral"]) expect(pages.get(path)?.auth).toBe(true);
  });
  it("advertises only maintained routes without hardcoded model or demo links", () => {
    const allowed=new Set(PUBLIC_PAGE_SPECS.map(page=>page.href));
    const links=[...FOOTER_GROUPS.flatMap(group=>group.links),...MEGA_MENUS.flatMap(menu=>menu.columns.flatMap(column=>column.links))];
    for(const link of links) expect(allowed.has(link.href.split("?")[0])).toBe(true);
    expect(links.some(link=>link.href.startsWith("/models/"))).toBe(false);
  });
});
