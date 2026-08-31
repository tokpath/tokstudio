import { describe, expect, it } from "vitest";
import { consoleItemHref, isConsolePath, isNavActive, userSections } from "./nav";

describe("isConsolePath", () => {
  it("treats app channel partner and admin as consoles", () => {
    expect(isConsolePath("/")).toBe(false);
    expect(isConsolePath("/docs")).toBe(false);
    expect(isConsolePath("/login")).toBe(false);
    expect(isConsolePath("/app")).toBe(true);
    expect(isConsolePath("/channel/users")).toBe(true);
    expect(isConsolePath("/partner")).toBe(true);
    expect(isConsolePath("/admin/plans")).toBe(true);
  });
});

describe("user console nav", () => {
  it("uses real routes instead of hash anchors", () => {
    expect(userSections.some((item) => item.href === "/app/keys")).toBe(true);
    expect(userSections.some((item) => item.href.startsWith("#"))).toBe(false);
  });

  it("highlights overview only on /app", () => {
    expect(isNavActive("/app", "/app")).toBe(true);
    expect(isNavActive("/app/keys", "/app")).toBe(false);
    expect(isNavActive("/app/keys", "/app/keys")).toBe(true);
    expect(isNavActive("/app/settings/team", "/app/settings")).toBe(true);
  });

  it("keeps hash prefixes for channel-style items", () => {
    expect(consoleItemHref({ href: "/app/wallet", label: "余额/充值" }, "/app")).toBe("/app/wallet");
    expect(consoleItemHref({ href: "#users", label: "本渠道用户" }, "/channel")).toBe("/channel#users");
  });
});
