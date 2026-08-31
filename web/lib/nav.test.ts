import { describe, expect, it } from "vitest";
import { channelSections, consoleItemHref, isConsolePath, isNavActive, userSections } from "./nav";

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

  it("uses real routes for channel console too", () => {
    expect(channelSections.some((item) => item.href === "/channel/users")).toBe(true);
    expect(channelSections.some((item) => item.href.startsWith("#"))).toBe(false);
    expect(isNavActive("/channel", "/channel")).toBe(true);
    expect(isNavActive("/channel/users", "/channel")).toBe(false);
  });

  it("keeps hash prefixes only when href is a hash", () => {
    expect(consoleItemHref({ href: "/app/wallet", key: "wallet" }, "/app")).toBe("/app/wallet");
    expect(consoleItemHref({ href: "#users", key: "users" }, "/channel")).toBe("/channel#users");
  });
});
