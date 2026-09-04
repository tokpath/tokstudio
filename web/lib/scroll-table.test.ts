import { describe, expect, it } from "vitest";
import { shouldStickyEnds, stickyColumnClass } from "@/lib/scroll-table";

describe("scroll-table sticky helpers", () => {
  it("pins ends only when there are enough columns", () => {
    expect(shouldStickyEnds(2)).toBe(false);
    expect(shouldStickyEnds(3)).toBe(true);
    expect(shouldStickyEnds(3, false)).toBe(false);
  });

  it("marks first and last columns sticky", () => {
    const first = stickyColumnClass(0, 4, { stickyEnds: true, header: true });
    const mid = stickyColumnClass(1, 4, { stickyEnds: true });
    const last = stickyColumnClass(3, 4, { stickyEnds: true });
    expect(first).toContain("sticky");
    expect(first).toContain("left-0");
    expect(mid).not.toContain("sticky");
    expect(last).toContain("sticky");
    expect(last).toContain("right-0");
  });
});
