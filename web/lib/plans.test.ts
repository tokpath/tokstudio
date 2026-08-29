import { describe, expect, it } from "vitest";

function formatPlanPrice(minor: number): string {
  return (minor / 1_000_000).toString();
}

describe("plan price display", () => {
  it("formats the Echo monthly seed price as 10 USD", () => {
    expect(formatPlanPrice(10_000_000)).toBe("10");
  });
});
