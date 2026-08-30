import { describe, expect, it } from "vitest";
import {
  PORTALS,
  PUBLIC_NAV,
  findPortalItem,
  listConsoleSectionParams,
  listPortalParams,
} from "./nav";

describe("DESIGN.md portal menus", () => {
  it("keeps the public nav as status/docs/models/pricing", () => {
    expect(PUBLIC_NAV.map((item) => item.label)).toEqual(["状态", "文档", "模型", "定价"]);
  });

  it("keeps the user console menu", () => {
    expect(PORTALS.user.items.map((item) => item.label)).toEqual([
      "总览",
      "余额/充值",
      "套餐",
      "API Key",
      "用量/账单",
      "媒体任务",
      "文档",
      "设置",
    ]);
  });

  it("keeps the channel console menu", () => {
    expect(PORTALS.channel.items.map((item) => item.label)).toEqual([
      "本渠道用户",
      "套餐",
      "推广",
      "额度",
      "用量",
      "佣金/结算",
    ]);
  });

  it("keeps the admin console menu", () => {
    expect(PORTALS.admin.items.map((item) => item.label)).toEqual([
      "总览",
      "提供商",
      "模型",
      "路由组",
      "价格",
      "用户",
      "余额",
      "用量",
      "渠道",
      "套餐审核",
      "佣金策略",
      "指标",
      "审计",
      "设置",
    ]);
  });

  it("resolves every generated console route", () => {
    const landings = listPortalParams();
    const sections = listConsoleSectionParams();
    expect(landings).toHaveLength(3);
    expect(landings.length + sections.length).toBe(
      PORTALS.user.items.length + PORTALS.channel.items.length + PORTALS.admin.items.length,
    );
    for (const entry of landings) {
      expect(findPortalItem(entry.portal as "user" | "channel" | "admin")).toBeDefined();
    }
    for (const entry of sections) {
      expect(
        findPortalItem(entry.portal as "user" | "channel" | "admin", entry.section),
        `${entry.portal}/${entry.section}`,
      ).toBeDefined();
    }
  });

  it("keeps portal heroes on the three landing pages", () => {
    expect(PORTALS.user.items[0]?.hero).toBe("user-overview");
    expect(PORTALS.channel.items[0]?.hero).toBe("channel-home");
    expect(PORTALS.admin.items[0]?.hero).toBe("admin-overview");
  });

  it("does not resolve unknown console slugs", () => {
    expect(findPortalItem("user", "cart")).toBeUndefined();
    expect(findPortalItem("admin", "providers/secret")).toBeUndefined();
  });
});
