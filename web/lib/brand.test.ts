import { describe, expect, it } from "vitest";
import { assetByteLimit, describeAssetLimit, publicAssetURL, themeStyle } from "./brand";

describe("themeStyle", () => {
  it("maps OEM stamp tokens", () => {
    const style = themeStyle({
      id: "oem",
      name: "Aurora OEM",
      primary_domain: "oem.localhost",
      api_domain: "api.oem.localhost",
      admin_domain: "admin.oem.localhost",
      theme: { brand: "#92400E", brand_emphasis: "#92400E" },
    });
    expect(style["--oem-brand"]).toBe("#92400E");
    expect(style["--oem-brand-emphasis"]).toBe("#92400E");
  });

  it("still accepts legacy primary key", () => {
    const style = themeStyle({
      id: "oem",
      name: "Aurora OEM",
      primary_domain: "oem.localhost",
      api_domain: "api.oem.localhost",
      admin_domain: "admin.oem.localhost",
      theme: { primary: "#f59e0b", background: "#111827" },
    });
    expect(style["--oem-brand"]).toBe("#f59e0b");
  });
});

describe("publicAssetURL", () => {
  it("prefixes same-origin API paths", () => {
    expect(publicAssetURL("/v1/public/brand-assets/bas_1")).toBe("/api/v1/public/brand-assets/bas_1");
    expect(publicAssetURL("https://cdn.example/logo.png")).toBe("https://cdn.example/logo.png");
  });
});

describe("asset limits (option E)", () => {
  it("uses 128KiB and 1024px for logo", () => {
    expect(assetByteLimit("logo")).toBe(128 * 1024);
    expect(describeAssetLimit("logo")).toContain("128KiB");
    expect(describeAssetLimit("logo")).toContain("1024");
  });
});
