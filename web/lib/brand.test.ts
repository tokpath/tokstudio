import { describe, expect, it } from "vitest";
import { themeStyle } from "./brand";

describe("themeStyle", () => {
  it("uses OEM theme tokens", () => {
    const style = themeStyle({
      id: "oem",
      name: "Aurora OEM",
      primary_domain: "oem.localhost",
      api_domain: "api.oem.localhost",
      admin_domain: "admin.oem.localhost",
      theme: { primary: "#f59e0b", background: "#111827" },
    });
    expect(style["--brand"]).toBe("#f59e0b");
  });
});
