import { describe, expect, it } from "vitest";
import { adminNavKeys, isConsolePath } from "./nav";

describe("adminNavKeys", () => {
  it("covers every admin palette label key", () => {
    expect(adminNavKeys).toEqual([
      "overview",
      "providers",
      "models",
      "routes",
      "keys",
      "plans",
      "prices",
      "payments",
      "billing",
      "usage",
      "channels",
      "promos",
      "commission",
      "metrics",
      "media",
      "users",
      "alerts",
      "runbooks",
      "audit",
      "settings",
    ]);
  });
});

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
