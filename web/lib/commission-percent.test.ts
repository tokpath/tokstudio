import { describe, expect, it } from "vitest";
import { bpsToPercent, percentToBps } from "./commission-percent";

describe("commission percentages", () => {
  it("preserves two decimal percentage precision", () => {
    expect(bpsToPercent(1500)).toBe("15");
    expect(percentToBps("2.51")).toBe(251);
    expect(percentToBps("0")).toBe(0);
  });
  it("rejects empty, excessive precision, negative and out of range input", () => {
    for (const raw of ["", "2.001", "-1", "101", "Infinity", "1e2"]) expect(percentToBps(raw)).toBeNull();
  });
});
