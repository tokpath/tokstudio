import { describe, expect, it } from "vitest";
import { formatUsdMinor, parseUsdToMinor } from "./money";

describe("usd minor boundary", () => {
  it("parses USD input into ledger minor without a million-times mental convert", () => {
    expect(parseUsdToMinor("1")).toBe(1_000_000);
    expect(parseUsdToMinor("$1.50")).toBe(1_500_000);
    expect(parseUsdToMinor("0.000001")).toBe(1);
    expect(parseUsdToMinor("0.004")).toBe(4_000);
    expect(parseUsdToMinor("")).toBeNull();
    expect(parseUsdToMinor("1.0000001")).toBeNull();
  });

  it("does not collapse a sub-cent charge to $0.00", () => {
    expect(formatUsdMinor(0)).toBe("$0.00");
    expect(formatUsdMinor(26)).toBe("$0.000026");
    expect(formatUsdMinor(4_000)).toBe("$0.004");
    expect(formatUsdMinor(10_000)).toBe("$0.01");
    expect(formatUsdMinor(1_000_000)).toBe("$1.00");
    expect(formatUsdMinor(26, "小于 $0.01")).toBe("小于 $0.01");
    expect(formatUsdMinor(undefined)).toBe("—");
    expect(formatUsdMinor("1000000")).toBe("$1.00");
  });
});
