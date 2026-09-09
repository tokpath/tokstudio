import { describe, expect, it } from "vitest";
import { adminGroups, portalLinks } from "./nav";
import {
  canAccessAdminConsole,
  canAccessChannelPortal,
  canAccessPartnerPortal,
  canViewAdminHref,
  canWrite,
  filterAdminGroups,
  filterPortalHrefs,
  type Viewer,
} from "./rbac";

function signed(roles: string[], extra: Partial<Viewer> = {}): Viewer {
  return { signedIn: true, loading: false, roles, ...extra };
}

const guest: Viewer = { signedIn: false, loading: false, roles: [] };

describe("role menus", () => {
  it("lets unsigned viewers keep the full admin nav", () => {
    expect(filterAdminGroups(adminGroups, guest).flatMap((group) => group.items).length).toBe(
      adminGroups.flatMap((group) => group.items).length,
    );
    expect(canViewAdminHref("/admin/providers", guest)).toBe(true);
    expect(canWrite("providers.write", guest)).toBe(true);
  });

  it("hides upstream keys and user bans from finance", () => {
    const finance = signed(["finance_admin"]);
    const hrefs = filterAdminGroups(adminGroups, finance).flatMap((group) => group.items.map((item) => item.href));
    expect(hrefs).toContain("/admin/billing");
    expect(hrefs).toContain("/admin/margin");
    expect(hrefs).toContain("/admin/channels");
    expect(hrefs).toContain("/admin/commission");
    expect(hrefs).not.toContain("/admin/providers");
    expect(hrefs).not.toContain("/admin/users");
    expect(hrefs).not.toContain("/admin/audit");
    expect(hrefs).not.toContain("/admin/keys");
    expect(hrefs).not.toContain("/admin/prices");
    expect(canWrite("billing.refund", finance)).toBe(true);
    expect(canWrite("providers.write", finance)).toBe(false);
    expect(canWrite("models.grant", finance)).toBe(false);
    expect(canWrite("channels.quota", finance)).toBe(true);
    expect(canWrite("prices.write", finance)).toBe(true);
  });

  it("lets ops manage catalog and grants, but not refunds or bans", () => {
    const ops = signed(["ops_admin"]);
    const hrefs = filterAdminGroups(adminGroups, ops).flatMap((group) => group.items.map((item) => item.href));
    expect(canViewAdminHref("/admin/models", ops)).toBe(true);
    expect(canViewAdminHref("/admin/channels", ops)).toBe(true);
    expect(canViewAdminHref("/admin/users", ops)).toBe(false);
    expect(canViewAdminHref("/admin/audit", ops)).toBe(false);
    expect(hrefs).not.toContain("/admin/keys");
    expect(hrefs).not.toContain("/admin/prices");
    expect(canWrite("models.grant", ops)).toBe(true);
    expect(canWrite("billing.bonus", ops)).toBe(true);
    expect(canWrite("billing.refund", ops)).toBe(false);
    expect(canWrite("commission.write", ops)).toBe(false);
    expect(canWrite("prices.write", ops)).toBe(true);
  });

  it("lets tech manage credentials but not billing writes", () => {
    const tech = signed(["tech_admin"]);
    expect(canViewAdminHref("/admin/providers", tech)).toBe(true);
    expect(canViewAdminHref("/admin/keys", tech)).toBe(false);
    expect(canViewAdminHref("/admin/billing", tech)).toBe(false);
    expect(canViewAdminHref("/admin/commission", tech)).toBe(false);
    expect(canViewAdminHref("/admin/channels", tech)).toBe(false);
    expect(canWrite("providers.write", tech)).toBe(true);
    expect(canWrite("billing.refund", tech)).toBe(false);
    expect(canWrite("commission.write", tech)).toBe(false);
  });

  it("keeps audit read-only", () => {
    const audit = signed(["audit_readonly"]);
    expect(canViewAdminHref("/admin/audit", audit)).toBe(true);
    expect(canViewAdminHref("/admin/billing", audit)).toBe(true);
    expect(canViewAdminHref("/admin/users", audit)).toBe(false);
    expect(canViewAdminHref("/admin/settings", audit)).toBe(false);
    expect(canWrite("audit.probe", audit)).toBe(false);
    expect(canWrite("billing.refund", audit)).toBe(false);
    expect(canWrite("billing.bonus", audit)).toBe(false);
    expect(canWrite("users.write", audit)).toBe(false);
  });

  it("scopes portals by role", () => {
    expect(canAccessAdminConsole(["finance_admin"])).toBe(true);
    expect(canAccessAdminConsole(["channel_admin"])).toBe(false);
    expect(canAccessChannelPortal(["channel_admin"])).toBe(true);
    expect(canAccessChannelPortal(["finance_admin"])).toBe(false);
    const finance = signed(["finance_admin"]);
    expect(canAccessPartnerPortal(finance)).toBe(false);
    expect(canAccessPartnerPortal(signed(["end_user"], { isPartner: true }))).toBe(true);
    const hrefs = filterPortalHrefs(
      portalLinks.map((item) => item.href),
      finance,
    );
    expect(hrefs).toContain("/admin");
    expect(hrefs).toContain("/app");
    expect(hrefs).not.toContain("/channel");
    expect(hrefs).not.toContain("/partner");
  });

  it("matches nested admin detail routes", () => {
    const finance = signed(["finance_admin"]);
    expect(canViewAdminHref("/admin/channels/chn_reseller_b", finance)).toBe(true);
    expect(canViewAdminHref("/admin/partners/acr_b_agent", finance)).toBe(false);
    expect(canViewAdminHref("/admin/models/tokenhub/echo-1", signed(["ops_admin"]))).toBe(true);
  });
});
