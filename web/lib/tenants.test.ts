import { describe, expect, it } from "vitest";
import {
  adminNavActive,
  canGrantTenantModels,
  channelHref,
  channelTypeLabel,
  channelUsesQuota,
  isKOLType,
  partnerHref,
  roleTypeLabel,
} from "./tenants";

describe("tenant list helpers", () => {
  it("labels channel orgs and acquisition roles separately", () => {
    expect(channelTypeLabel("A")).toBe("A 平台直推");
    expect(channelTypeLabel("B")).toBe("B 批发商");
    expect(channelTypeLabel("C")).toBe("C OEM");
    expect(roleTypeLabel("agent")).toBe("代理商");
    expect(roleTypeLabel("kol_l1")).toBe("1 级 KOL");
    expect(roleTypeLabel("kol_l2")).toBe("2 级 KOL");
    expect(isKOLType("agent")).toBe(false);
    expect(isKOLType("kol_l2")).toBe(true);
    expect(channelUsesQuota("A")).toBe(false);
    expect(channelUsesQuota("B")).toBe(true);
    expect(canGrantTenantModels(["platform_admin"])).toBe(true);
    expect(canGrantTenantModels(["ops_admin"])).toBe(true);
    expect(canGrantTenantModels(["channel_admin"])).toBe(false);
    expect(canGrantTenantModels(undefined)).toBe(false);
  });

  it("builds detail hrefs and highlights nested admin nav", () => {
    expect(channelHref("chn_reseller_b")).toBe("/admin/channels/chn_reseller_b");
    expect(partnerHref("acr_b_agent")).toBe("/admin/partners/acr_b_agent");
    expect(adminNavActive("/admin", "/admin")).toBe(true);
    expect(adminNavActive("/admin/channels", "/admin")).toBe(false);
    expect(adminNavActive("/admin/channels/chn_oem_c", "/admin/channels")).toBe(true);
    expect(adminNavActive("/admin/partners/acr_b_kol1", "/admin/channels")).toBe(true);
    expect(adminNavActive("/admin/providers", "/admin/channels")).toBe(false);
  });
});
