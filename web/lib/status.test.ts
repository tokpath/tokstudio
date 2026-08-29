import { describe, expect, it } from "vitest";
import { summarizeReady } from "./status";

describe("summarizeReady", () => {
  it("explains a ready control plane", () => {
    expect(summarizeReady({ status: "ready", checks: { postgres: "ok" } })).toContain("已就绪");
  });

  it("lists failed checks", () => {
    expect(summarizeReady({ status: "not_ready", checks: { redis: "error" } })).toContain("redis");
  });
});
