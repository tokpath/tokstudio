import { describe, expect, it } from "vitest";
import {
  activityApiQuery,
  activityHref,
  activityTimeWindow,
  dimFilterKeys,
  isInHalfOpen,
  mergeFilterValues,
  parseActivitySearchParams,
  resolveActivityWindow,
} from "./activity-query";

const shanghai = "Asia/Shanghai";
const morning = new Date("2026-09-15T17:00:00.000Z"); // 2026-09-16 01:00 +08
const noon = new Date("2026-09-16T04:00:00.000Z"); // 2026-09-16 12:00 +08
const sep16Start = "2026-09-15T16:00:00.000Z";
const sep17Start = "2026-09-16T16:00:00.000Z";

describe("activity query", () => {
  it("round-trips filters in the user-facing URL", () => {
    const href = activityHref({ range: "7d", model: "tokenhub/echo-1", result: "failed", billing: "voided", key: "key_1" });
    expect(href).toBe("/app/activity?range=7d&model=tokenhub%2Fecho-1&result=failed&billing=voided&key=key_1");
    const parsed = parseActivitySearchParams(new URL(href, "https://tokenhub.local").searchParams);
    expect(parsed).toEqual({
      range: "7d",
      from: undefined,
      to: undefined,
      model: "tokenhub/echo-1",
      result: "failed",
      billing: "voided",
      key: "key_1",
    });
  });

  it("uses the local calendar day for today at Shanghai 01:00, not the UTC date", () => {
    const window = activityTimeWindow({ range: "today" }, morning, shanghai);
    expect(window.error).toBeUndefined();
    expect(window.from).toBe(sep16Start);
    expect(window.to).toBe(sep17Start);
    expect(isInHalfOpen(morning, window.from, window.to)).toBe(true);
    expect(isInHalfOpen(new Date("2026-09-15T15:59:59.000Z"), window.from, window.to)).toBe(false);
  });

  it("keeps the same local day window at Shanghai noon", () => {
    const window = activityTimeWindow({ range: "today" }, noon, shanghai);
    expect(window.from).toBe(sep16Start);
    expect(window.to).toBe(sep17Start);
    expect(isInHalfOpen(noon, window.from, window.to)).toBe(true);
  });

  it("includes the last instant of a same-day custom range and excludes the next local midnight", () => {
    const window = activityTimeWindow({ range: "custom", from: "2026-09-16", to: "2026-09-16" }, noon, shanghai);
    expect(window.from).toBe(sep16Start);
    expect(window.to).toBe(sep17Start);
    expect(isInHalfOpen(new Date("2026-09-16T15:59:59.999Z"), window.from, window.to)).toBe(true);
    expect(isInHalfOpen(new Date("2026-09-16T16:00:00.000Z"), window.from, window.to)).toBe(false);
  });

  it("rejects reversed and impossible civil dates without calling them a valid window", () => {
    expect(activityTimeWindow({ range: "custom", from: "2026-09-17", to: "2026-09-16" }, noon, shanghai).error).toBe("order");
    expect(activityTimeWindow({ range: "custom", from: "2026-02-31", to: "2026-03-01" }, noon, shanghai).error).toBe("invalid");
    const params = activityApiQuery({ range: "custom", from: "2026-09-17", to: "2026-09-16" }, noon, shanghai);
    expect(params.get("from")).toBeNull();
    expect(params.get("to")).toBeNull();
  });

  it("maps URL filters onto RFC3339 half-open bounds for the requests API", () => {
    const now = new Date("2026-09-16T12:00:00.000Z");
    const today = resolveActivityWindow({ range: "today" }, now, "UTC");
    expect(today).toEqual({ from: "2026-09-16T00:00:00.000Z", to: "2026-09-17T00:00:00.000Z" });
    const week = resolveActivityWindow({ range: "7d" }, now, "UTC");
    expect(week).toEqual({ from: "2026-09-09T00:00:00.000Z", to: "2026-09-17T00:00:00.000Z" });
    const params = activityApiQuery({ range: "7d", model: "tokenhub/echo-1", result: "failed", billing: "confirmed" }, now, "UTC");
    expect(params.get("limit")).toBe("100");
    expect(params.get("from")).toBe("2026-09-09T00:00:00.000Z");
    expect(params.get("to")).toBe("2026-09-17T00:00:00.000Z");
    expect(params.get("public_model_id")).toBe("tokenhub/echo-1");
    expect(params.get("result")).toBe("failed");
    expect(params.get("billing_state")).toBe("confirmed");
    expect(params.get("state")).toBeNull();
  });

  it("reads legacy status only when it is a real result or billing state", () => {
    expect(parseActivitySearchParams(new URLSearchParams("status=failed")).result).toBe("failed");
    expect(parseActivitySearchParams(new URLSearchParams("status=voided")).billing).toBe("voided");
    expect(parseActivitySearchParams(new URLSearchParams("status=pending")).result).toBeUndefined();
    expect(parseActivitySearchParams(new URLSearchParams("status=pending")).billing).toBeUndefined();
  });

  it("keeps filter options from dims even when the page only has one row", () => {
    expect(
      mergeFilterValues(
        dimFilterKeys([{ key: "tokenhub/echo-1" }, { key: "tokenhub/other" }, { key: "" }]),
        ["tokenhub/other"],
        ["tokenhub/echo-1"],
      ),
    ).toEqual(["tokenhub/echo-1", "tokenhub/other"]);
  });

  it("keeps custom dates only when range is custom", () => {
    expect(activityHref({ range: "7d", from: "2026-01-01" })).toBe("/app/activity?range=7d");
    expect(activityHref({ range: "custom", from: "2026-09-01", to: "2026-09-16" })).toBe(
      "/app/activity?range=custom&from=2026-09-01&to=2026-09-16",
    );
  });
});
