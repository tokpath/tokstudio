import { describe, expect, it } from "vitest";
import { canReadResource, customerSections, operatorNav, operatorLanding, operatorResourcePath, sectionForResource, staffNotificationPath, visibleOperatorSections } from "./navigation";

describe("task navigation and organization boundaries", () => {
  it("keeps every established operator capability reachable once without duplicate risk or content storage settings", () => {
    const resources = operatorNav.flatMap(section => section.items.map(([resource]) => resource));
    expect(resources).toHaveLength(44);
    expect(new Set(resources).size).toBe(44);
    expect(resources).not.toContain("withdrawals");
    expect(resources).toContain("routes");
    expect(resources).toContain("payment-quotes");
    expect(resources).toContain("pricing");
    expect(resources).not.toContain("risk");
    expect(resources).not.toContain("retention");
    expect(sectionForResource(operatorNav,"approvals")?.id).toBe("finance");
    expect(resources).not.toContain("routing");
    expect(resources).not.toContain("catalog-changes");
    expect(resources).not.toContain("takeover-settlements");
    expect(customerSections).toHaveLength(7);
    for(const route of ["topups","credit-sources","refunds","invoices","activities","notifications","orders"]) expect(sectionForResource(customerSections, route)).toBeDefined();
  });
  it("does not grant a parent workspace any permission to downstream business", () => {
    expect(canReadResource({operator:{kind:"oem"},permissions:["organization.read","operators.read"]}, "customers")).toBe(false);
    expect(visibleOperatorSections({operator:{kind:"oem"},permissions:["operators.read"]})).toHaveLength(1);
  });
  it("keeps platform-only and parent-only tasks out of channel navigation even for owners", () => {
    const channel = {operator:{kind:"channel"},permissions:["*"]};
    for (const route of ["catalog","providers","routes","requests","jobs","payment-assets","operators","channel-themes","takeovers"]) expect(canReadResource(channel, route)).toBe(false);
    expect(canReadResource(channel,"mail")).toBe(true);
    expect(sectionForResource(operatorNav,"mail")?.id).toBe("settings");
    expect(canReadResource({operator:{kind:"oem"},permissions:["mail.read"]},"mail")).toBe(true);
    expect(canReadResource(channel,"procurement")).toBe(true);
    expect(canReadResource(channel,"pricing")).toBe(true);
  });
  it("points the header bell at the current workspace when that workspace can read notifications", () => {
    const workspaces = [
      { operator: { id: "one", kind: "platform" }, permissions: ["notifications.read"] },
      { operator: { id: "two", kind: "channel" }, permissions: ["payments.read"] },
    ];
    expect(staffNotificationPath(workspaces, "/manage/one/payments")).toBe("/manage/one/notifications");
    expect(staffNotificationPath(workspaces, "/manage/two/payments")).toBeUndefined();
    expect(staffNotificationPath(workspaces, "/models", "two")).toBe("/manage/one/notifications");
  });
  it("lands staff on an authorized task instead of an inaccessible overview", () => {
    expect(operatorLanding({operator:{id:"finance",kind:"oem"},permissions:["payments.read"]})).toBe("/manage/finance/payments");
    expect(sectionForResource(operatorNav,"refunds/record")?.id).toBe("finance");
  });
  it("gives payout staff the combined entry without granting commission rule access", () => {
    const finance = {operator:{id:"finance",kind:"oem"},permissions:["withdrawals.read", "withdrawals.manage"]};
    expect(canReadResource(finance,"commissions")).toBe(false);
    expect(visibleOperatorSections(finance).flatMap(group=>group.items)).toEqual([["commissions","邀请与分佣"]]);
    expect(operatorLanding(finance)).toBe("/manage/finance/commissions");
    expect(operatorResourcePath("finance","withdrawals")).toBe("/manage/finance/commissions?section=requests");
    expect(sectionForResource(customerSections,"withdrawals")?.id).toBe("promotion");
  });
});
