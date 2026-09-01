import { describe, expect, it } from "vitest";
import { portalForPath, themeStyle } from "./brand";

describe("portalForPath", () => {
  it("maps the four portals plus docs", () => {
    expect(portalForPath("/")).toBe("public");
    expect(portalForPath("/app")).toBe("user");
    expect(portalForPath("/console")).toBe("user");
    expect(portalForPath("/channel/users")).toBe("channel");
    expect(portalForPath("/partner")).toBe("partner");
    expect(portalForPath("/admin")).toBe("admin");
    expect(portalForPath("/docs")).toBe("docs");
  });
});

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
    expect(style["--brand-primary"]).toBe("#f59e0b");
  });
});
