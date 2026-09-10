import { describe, expect, it } from "vitest";
import zh from "../messages/zh.json";
import {
  COST_SOURCE,
  correctionKinds,
  costSourceLabel,
  emptyAttemptCostTitle,
  forbidsEstimateCost,
  isAdminOnlyMarginPath,
  isNegativeMargin,
  marginClassName,
  marginMinor,
  pageTitle,
} from "./margin";
import { userSections, channelSections, adminNavKeys } from "./nav";

describe("W-meter ④ margin assembly", () => {
  it("computes 毛利 = 售 − 成本 and keeps negatives as danger", () => {
    expect(marginMinor(100, 40)).toBe(60);
    expect(marginMinor(40, 100)).toBe(-60);
    expect(isNegativeMargin(-1)).toBe(true);
    expect(marginClassName(-12)).toContain("--danger");
    expect(marginClassName(12)).not.toContain("--danger");
  });

  it("pins TokenHub as the only cost source and uses the locked empty title", () => {
    expect(costSourceLabel()).toBe("TokenHub");
    expect(COST_SOURCE).toBe("TokenHub");
    expect(emptyAttemptCostTitle()).toBe("暂无 attempt 成本");
    expect(pageTitle()).toBe("成本/毛利");
    expect(zh.admin.margin).toBe("成本/毛利");
    expect(zh.adminUi.margin).toBe("成本/毛利");
    expect(zh.adminUi.marginEmpty).toBe("暂无 attempt 成本");
  });

  it("is admin-only: no user or channel portal entry", () => {
    expect(isAdminOnlyMarginPath("/admin/margin")).toBe(true);
    expect(adminNavKeys).toContain("margin");
    expect(userSections.some((item) => item.href === "/admin/margin" || item.key === "margin")).toBe(false);
    expect(channelSections.some((item) => item.href === "/admin/margin" || item.key === "margin")).toBe(false);
    expect(userSections.some((item) => item.href === "/app/margin")).toBe(false);
    expect(channelSections.some((item) => item.href === "/channel/margin")).toBe(false);
  });

  it("forbids estimate fill and only offers sealed correction tickets", () => {
    expect(correctionKinds()).toEqual(["fill_cost", "adjust_margin"]);
    const labels = [zh.adminUi.fillCost, zh.adminUi.adjustMargin, zh.adminUi.marginLead];
    expect(forbidsEstimateCost(labels)).toBe(true);
    expect(labels.join(" ")).not.toMatch(/估扣|estimate-debit|手填成本/i);
  });
});
