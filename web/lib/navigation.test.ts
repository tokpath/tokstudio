import { describe, expect, it } from "vitest";
import { canReadResource, customerSections, operatorNav, operatorLanding, sectionForResource, staffNotificationPath, visibleOperatorSections } from "./navigation";

describe("task navigation and organization boundaries", () => {
  it("keeps every established operator capability reachable exactly once", () => {
    const resources = operatorNav.flatMap(section => section.items.map(([resource]) => resource));
    expect(resources).toHaveLength(51);
    expect(new Set(resources).size).toBe(51);
    expect(customerSections).toHaveLength(7);
    for(const route of ["topups","credit-sources","refunds","invoices","activities","notifications","orders"]) expect(sectionForResource(customerSections, route)).toBeDefined();
  });
  it("does not grant a parent workspace any permission to downstream business", () => {
    expect(canReadResource({operator:{kind:"oem"},permissions:["organization.read","operators.read"]}, "customers")).toBe(false);
    expect(visibleOperatorSections({operator:{kind:"oem"},permissions:["operators.read"]})).toHaveLength(1);
  });
  it("keeps platform-only and parent-only tasks out of channel navigation even for owners", () => {
    const channel = {operator:{kind:"channel"},permissions:["*"]};
    for (const route of ["catalog","providers","routes","requests","jobs","mail","payment-assets","operators","channel-themes","takeovers"]) expect(canReadResource(channel, route)).toBe(false);
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
});
