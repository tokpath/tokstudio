import { describe, expect, it } from "vitest";
import {
  badgeForCheck,
  badgeForReady,
  classifyCheck,
  formatControlReceipt,
  listChecks,
  summarizeReady,
} from "./status";

describe("summarizeReady", () => {
  it("explains a ready control plane", () => {
    expect(summarizeReady({ status: "ready", checks: { postgres: "ok" } })).toContain("已就绪");
  });

  it("lists failed checks", () => {
    expect(summarizeReady({ status: "not_ready", checks: { redis: "error" } })).toContain("redis");
  });
});

describe("classifyCheck", () => {
  it("maps upstream values to ledger states", () => {
    expect(classifyCheck("ok")).toBe("ok");
    expect(classifyCheck("stale")).toBe("stale");
    expect(classifyCheck("error")).toBe("error");
    expect(classifyCheck(undefined)).toBe("unknown");
  });
});

describe("badgeForCheck", () => {
  it("always pairs a word with a tone", () => {
    expect(badgeForCheck("ok")).toEqual({ word: "READY", tone: "success" });
    expect(badgeForCheck("stale")).toEqual({ word: "DEGRADED", tone: "degraded" });
    expect(badgeForCheck("error")).toEqual({ word: "UNAVAILABLE", tone: "danger" });
    expect(badgeForCheck("unknown")).toEqual({ word: "HOLD", tone: "hold" });
  });
});

describe("badgeForReady", () => {
  it("treats a fetch failure as a hold, not an unknown gray state", () => {
    expect(badgeForReady({ error: "无法连接 API" })).toEqual({ word: "HOLD", tone: "hold" });
  });

  it("uses unavailable when any check is down", () => {
    expect(
      badgeForReady({ status: "not_ready", checks: { postgres: "ok", redis: "error" } }).word,
    ).toBe("UNAVAILABLE");
  });
});

describe("formatControlReceipt", () => {
  it("writes an explainable hop list", () => {
    const line = formatControlReceipt({
      status: "ready",
      request_id: "req_1",
      checks: {
        postgres: "ok",
        redis: "ok",
        migrations: "ok",
        outbox_worker: "ok",
      },
    });
    expect(line).toContain("postgres:ok");
    expect(line).toContain("READY");
    expect(line).toContain("req_1");
  });
});

describe("listChecks", () => {
  it("keeps the control-plane order", () => {
    const keys = listChecks({
      status: "ready",
      checks: { outbox_worker: "ok", redis: "ok", postgres: "ok", migrations: "ok" },
    }).map((row) => row.key);
    expect(keys).toEqual(["postgres", "redis", "migrations", "outbox_worker"]);
  });
});
