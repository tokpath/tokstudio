import { describe, expect, it } from "vitest";
import { adminGroups, adminNavKeys, availableNavItems, channelNavGroupsFor, channelNavItemForPath, channelSections, consoleItemHref, isAuthPath, isConsolePath, isNavActive, navItemForPath, userSections, userSettingsNav } from "./nav";
import { adminNavActive } from "./tenants";

describe("adminNavKeys", () => {
  it("covers every admin palette label key", () => {
    expect(adminNavKeys).toContain("models");
    expect(adminNavKeys).toContain("users");
    expect(adminNavKeys).not.toContain("keys");
    expect(adminNavKeys).not.toContain("runbooks");
  });
});

describe("channel keys nav", () => {
  it("gives OEM an independent task navigation without technical controls", () => {
    const oem = channelNavGroupsFor("C").flatMap(group => group.items);
    expect(oem.some(item => ['providers', 'routes'].includes(item.key))).toBe(false);
    expect(oem.every(item => item.href.startsWith("/channel"))).toBe(true);
    expect(channelNavItemForPath("/channel/models/model-1", "C")?.key).toBe("models");
    expect(channelNavItemForPath("/channel/keys", "C")?.key).toBe("users");
    expect(channelNavItemForPath("/channel/brand", "C")?.key).toBe("brand");
    expect(channelNavItemForPath("/channel/settlements", "C")?.key).toBe("commission");
  });
  it("keeps brand management with OEM and user operations with B", () => {
    const b = channelNavGroupsFor("B").flatMap((group) => group.items.map((item) => item.href));
    const c = channelNavGroupsFor("C").flatMap((group) => group.items.map((item) => item.href));
    expect(b).toContain("/channel/users");
    expect(b).not.toContain("/channel/plans");
    expect(b).not.toContain("/channel/payments");
    expect(b).not.toContain("/channel/ledger");
    expect(b).not.toContain("/channel/brand");
    expect(b).not.toContain("/channel/subchannels");
    expect(c).toContain("/channel/plans");
    expect(c).toContain("/channel/subchannels");
  });
  it("lists channel API keys after users", () => {
    expect(channelSections.some((item) => item.href === "/channel/keys")).toBe(false);
  });

  it("keeps funding and rules out of channel navigation", () => {
    const items = channelSections.map(item => item.href);
    expect(items).not.toContain("/channel/ledger");
    expect(items).not.toContain("/channel/rules");
    expect(items).not.toContain("/channel/payments");
    expect(items).toContain("/channel/commissions");
  });
});

describe("isConsolePath", () => {
  it("treats app channel partner and admin as consoles", () => {
    expect(isConsolePath("/")).toBe(false);
    expect(isConsolePath("/docs")).toBe(false);
    expect(isConsolePath("/login")).toBe(false);
    expect(isConsolePath("/app")).toBe(true);
    expect(isConsolePath("/console")).toBe(true);
    expect(isConsolePath("/channel/users")).toBe(true);
    expect(isConsolePath("/partner")).toBe(true);
    expect(isConsolePath("/admin/plans")).toBe(true);
  });
});

describe("isAuthPath", () => {
  it("treats login and console entry as chrome-free auth", () => {
    expect(isAuthPath("/login")).toBe(true);
    expect(isAuthPath("/login/oauth/google")).toBe(true);
    expect(isAuthPath("/enter")).toBe(true);
    expect(isAuthPath("/login?next=%2Fapp")).toBe(false);
    expect(isAuthPath("/enter?x=1")).toBe(false);
    expect(isAuthPath("/")).toBe(false);
    expect(isAuthPath("/docs")).toBe(false);
    expect(isAuthPath("/app")).toBe(false);
  });
});

describe("adminNavActive", () => {
  it("keeps overview exact and channels nested", () => {
    expect(adminNavActive("/admin", "/admin")).toBe(true);
    expect(adminNavActive("/admin/channels", "/admin")).toBe(false);
    expect(adminNavActive("/admin/partners/acr_b_agent", "/admin/channels")).toBe(true);
  });
});

describe("user console nav", () => {
  it("uses real routes instead of hash anchors", () => {
    expect(userSections.some((item) => item.href === "/app/keys")).toBe(true);
    expect(userSections.some((item) => item.href === "/app/reconciliation")).toBe(false);
    expect(userSections.some((item) => item.href.startsWith("#"))).toBe(false);
  });

  it("does not add profile to the frozen sidebar", () => {
    expect(userSections.some((item) => item.href === "/app/profile")).toBe(false);
  });

  it("highlights overview only on /app", () => {
    expect(isNavActive("/app", "/app")).toBe(true);
    expect(isNavActive("/app/keys", "/app")).toBe(false);
    expect(isNavActive("/app/keys", "/app/keys")).toBe(true);
    expect(isNavActive("/app/settings/team", "/app/settings")).toBe(true);
  });

  it("marks placeholder settings as unavailable without dropping their href", () => {
    expect(userSettingsNav.some((item) => item.href === "/app/settings/team" && item.unavailable)).toBe(true);
    expect(availableNavItems(userSettingsNav).map((item) => item.href)).toEqual([
      "/app/settings",
      "/app/settings/billing",
      "/app/settings/quotas",
    ]);
    expect(navItemForPath("/app/settings/team", userSettingsNav)?.key).toBe("team");
  });

  it("uses real routes for channel console too", () => {
    expect(channelSections.some((item) => item.href === "/channel/users")).toBe(true);
    expect(channelSections.some((item) => item.href === "/channel/payments")).toBe(false);
    expect(channelSections.some((item) => item.href === "/channel/models")).toBe(true);
    expect(channelSections.some((item) => item.href.startsWith("#"))).toBe(false);
    expect(isNavActive("/channel", "/channel")).toBe(true);
    expect(isNavActive("/channel/users", "/channel")).toBe(false);
  });

  it("keeps hash prefixes only when href is a hash", () => {
    expect(consoleItemHref({ href: "/app/wallet", key: "wallet" }, "/app")).toBe("/app/wallet");
    expect(consoleItemHref({ href: "#users", key: "users" }, "/channel")).toBe("/channel#users");
  });
});
