import { describe, expect, it } from "vitest";
import { fundingPreview, fundingSuggestion } from "./customer-finance";

describe("customer payment source allocation", () => {
  it("preserves 24 decimal places and large amounts without Number conversion", () => {
    expect(fundingSuggestion("9007199254740993.000000000000000000000001", "9007199254740993", "0.000000000000000000000001", true)).toEqual({ balance: "9007199254740993", commission: "0.000000000000000000000001", external: "0" });
    expect(fundingPreview("10.000000000000000001", "3.000000000000000001", "2.000000000000000001")).toEqual({ external: "4.999999999999999999" });
  });

  it("never suggests spending commission without explicit opt-in", () => {
    expect(fundingSuggestion("10", "3", "100")).toEqual({ balance: "3", commission: "0", external: "7" });
    expect(fundingSuggestion("10", "3", "100", true)).toEqual({ balance: "3", commission: "7", external: "0" });
    expect(fundingSuggestion("0", "3", "100", true)).toEqual({ balance: "0", commission: "0", external: "0" });
  });

  it("checks exact availability and prevents over-allocation", () => {
    expect(fundingPreview("1", "0.500000000000000000000001", "0", "0.5", "1").error).toContain("余额超过");
    expect(fundingPreview("1", "0", "0.500000000000000000000001", "1", "0.5").error).toContain("佣金超过");
    expect(fundingPreview("1", "0.5", "0.500000000000000000000001").error).toContain("合计超过");
  });

  it.each(["-1", "1e-24", "NaN", "01", ".5", "0.1234567890123456789012345"])("rejects malformed or over-precision amounts: %s", value => {
    expect(fundingPreview("10", value, "0").error).toContain("最多 24 位小数");
  });
});
