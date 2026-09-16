import { describe, expect, it } from "vitest";
import { adminNavKeys, availableNavItems, channelSections, consoleItemHref, isAuthPath, isConsolePath, isNavActive, navItemForPath, userSections, userSettingsNav } from "./nav";
import { adminNavActive } from "./tenants";

describe("adminNavKeys", () => {
  it("covers every admin palette label key", () => {
    expect(adminNavKeys).toEqual([
      "overview",
      "providers",
      "models",
      "routes",
      "plans",
      "payments",
      "billing",
      "usage",
      "margin",
      "channels",
      "brands",
      "promos",
      "commission",
      "metrics",
      "reconciliation",
      "media",
      "users",
      "alerts",
      "runbooks",
      "audit",
      "settings",
    ]);
  });
});

describe("channel keys nav", () => {
  it("lists channel API keys after users", () => {
    expect(channelSections.some((item) => item.href === "/channel/keys")).toBe(true);
  });

  it("lists channel stock and rules in the ledger group", () => {
    expect(channelSections.some((item) => item.href === "/channel/ledger")).toBe(true);
    expect(channelSections.some((item) => item.href === "/channel/rules")).toBe(true);
    expect(channelSections.some((item) => item.href === "/channel/reconciliation")).toBe(true);
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
    expect(userSections.some((item) => item.href === "/app/reconciliation")).toBe(true);
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
    expect(channelSections.some((item) => item.href === "/channel/payments")).toBe(true);
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
