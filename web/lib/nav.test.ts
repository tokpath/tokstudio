import { describe, expect, it } from "vitest";
import { isConsolePath } from "./nav";
import { adminNavActive } from "./tenants";

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

describe("adminNavActive", () => {
  it("keeps overview exact and channels nested", () => {
    expect(adminNavActive("/admin", "/admin")).toBe(true);
    expect(adminNavActive("/admin/channels", "/admin")).toBe(false);
    expect(adminNavActive("/admin/partners/acr_b_agent", "/admin/channels")).toBe(true);
  });
});
