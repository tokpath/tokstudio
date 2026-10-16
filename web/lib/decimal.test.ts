import { describe, expect, it } from "vitest";
import { compareDecimal, formatDecimal, previewDecimal, safeAssetURL } from "./decimal";
import { Decimal, CreditsSchema, ThemeSchema } from "./contracts";
import { resolveTheme } from "./theme";
describe("asset amounts and theme boundaries", () => {
  it("preserves micro-balances and values larger than Number can represent", () => {
    expect(formatDecimal("10000000000000000000000.000000000000000001")).toBe("10,000,000,000,000,000,000,000.000000000000000001");
    expect(compareDecimal("0.000000000000000001", "0")).toBe(1);
    expect(compareDecimal("9007199254740993.01", "9007199254740992.99")).toBe(1);
    expect(compareDecimal("1.00", "1")).toBe(0);
    expect(formatDecimal("-0.000000000000000001")).toBe("−0.000000000000000001");
  });
  it("summarizes presentation precision without floating point or erasing tiny assets",()=>{
    expect(previewDecimal("99.957499999999999999")).toBe("≈99.9575");
    expect(previewDecimal("0.000000000000000001")).toBe("<0.000001");
    expect(previewDecimal("-0.000000000000000001")).toBe("−<0.000001");
    expect(previewDecimal("9007199254740993.999999999")).toBe("≈9,007,199,254,740,994");
    expect(previewDecimal("0.017000000")).toBe("0.017");
  });
  it("rejects floating-point API amounts and scientific notation", () => {
    expect(Decimal.safeParse(0.01).success).toBe(false);
    expect(Decimal.safeParse("1e-9").success).toBe(false);
    expect(Decimal.safeParse("-1").success).toBe(false);
    expect(CreditsSchema.safeParse({ customer_atg: { available: 100, held: "0" } }).success).toBe(false);
  });
  it("inherits theme fields independently and refuses executable URLs/CSS", () => {
    expect(resolveTheme({ primary_color: "#112233", logo_url: "/brand.png" }, { primary_color: "#abcdef", logo_url: null })).toEqual({ primary_color: "#abcdef", accent_color: undefined, logo_url: "/brand.png" });
    expect(ThemeSchema.safeParse({ primary_color: "url(https://bad.test)" }).success).toBe(false);
    expect(safeAssetURL("javascript:alert(1)")).toBeUndefined();
    expect(safeAssetURL("//other.test/x")).toBeUndefined();
    expect(safeAssetURL("/logo.png")).toBe("/logo.png");
  });
});
