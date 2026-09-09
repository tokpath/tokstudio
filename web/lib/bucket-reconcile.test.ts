import { describe, expect, it } from "vitest";
import zh from "../messages/zh.json";
import {
  emptyUsageTitle,
  flagPath,
  forbidsEstimateDebit,
  isMatchRow,
  pageTitle,
  pendingQueueTitle,
  reconcileActions,
  rowTone,
} from "./bucket-reconcile";

describe("Aura three-bucket reconcile copy", () => {
  it("uses 对账 for map/h1 and 待对账队列 for the section, never a bare 待对账 heading", () => {
    expect(pageTitle()).toBe("对账");
    expect(zh.userNav.reconciliation).toBe("对账");
    expect(zh.channelNav.reconciliation).toBe("对账");
    expect(zh.console.reconciliation.title).toBe("对账");
    expect(zh.console.channelReconciliation.title).toBe("对账");
    expect(pendingQueueTitle()).toBe("待对账队列");
    expect(zh.reconcile.pendingList).toBe("待对账队列");
    expect(zh.userNav.reconciliation).not.toBe("待对账");
    expect(zh.console.reconciliation.title).not.toBe("待对账");
  });

  it("keeps an honest empty title with no fake charts", () => {
    expect(emptyUsageTitle()).toBe("暂无 usage");
    expect(zh.reconcile.empty).toBe("暂无 usage");
  });
});

describe("sentinel match and mismatch", () => {
  it("sends matching rows down the green OK path with no action", () => {
    const row = { request_id: "req_ok", match: true, status: "match", usage_minor: 160000, charge_minor: 160000 };
    expect(isMatchRow(row)).toBe(true);
    expect(rowTone(row)).toBe("success");
    expect(reconcileActions(row)).toEqual([]);
  });

  it("routes mismatches into pending_reconciliation and never silently drops them", () => {
    const row = { request_id: "req_gap", match: false, status: "mismatch", usage_minor: 0, charge_minor: 0 };
    expect(isMatchRow(row)).toBe(false);
    expect(rowTone(row)).toBe("danger");
    expect(reconcileActions(row)).toEqual(["flag_pending"]);
  });

  it("does not offer estimate or blind debit", () => {
    const labels = [zh.reconcile.flag, zh.reconcile.flagTitle, zh.reconcile.matchOk];
    expect(forbidsEstimateDebit(labels)).toBe(true);
    expect(labels.join(" ")).not.toMatch(/估扣按钮|estimate-debit|blind debit/i);
  });
});

describe("reconcile paths", () => {
  it("keeps user and channel endpoints isomorphic", () => {
    expect(flagPath("user")).toBe("/v1/me/reconciliation/flag");
    expect(flagPath("channel")).toBe("/channel/reconciliation/flag");
  });
});
