import { describe, expect, it } from "vitest";
import { keysCreateQueryOpen, overviewHasUsage, overviewNeedsTopup } from "./overview-guide";

describe("overviewHasUsage", () => {
  it("treats an empty or missing list as a first-run account", () => {
    expect(overviewHasUsage([])).toBe(false);
    expect(overviewHasUsage(undefined)).toBe(false);
    expect(overviewHasUsage([{ request_id: "req_1" }])).toBe(true);
  });
});

describe("overviewNeedsTopup", () => {
  it("only flags a known non-positive available balance", () => {
    expect(overviewNeedsTopup(undefined)).toBe(false);
    expect(overviewNeedsTopup("")).toBe(false);
    expect(overviewNeedsTopup("—")).toBe(false);
    expect(overviewNeedsTopup("0")).toBe(true);
    expect(overviewNeedsTopup("0.00")).toBe(true);
    expect(overviewNeedsTopup("1.25")).toBe(false);
  });
});

describe("keysCreateQueryOpen", () => {
  it("opens create only when create=1 is present", () => {
    expect(keysCreateQueryOpen("")).toBe(false);
    expect(keysCreateQueryOpen("?tab=list")).toBe(false);
    expect(keysCreateQueryOpen("?create=1")).toBe(true);
    expect(keysCreateQueryOpen("create=1")).toBe(true);
  });
});
