import { describe, expect, it } from "vitest";

function formatUSD(minor: number): string {
  return (minor / 1_000_000).toString();
}

describe("wallet display", () => {
  it("formats micro-USD without float-looking noise for whole dollars", () => {
    expect(formatUSD(10_000_000)).toBe("10");
    expect(formatUSD(16)).toBe("0.000016");
  });
});
