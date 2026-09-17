import { describe, expect, it } from "vitest";
import {
  activityApiQuery,
  activityHref,
  dimFilterKeys,
  mergeFilterValues,
  parseActivitySearchParams,
  resolveActivityWindow,
} from "./activity-query";

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

  it("maps URL filters onto the requests API without stuffing failed into billing state", () => {
    const now = new Date("2026-09-16T12:00:00.000Z");
    expect(resolveActivityWindow({ range: "today" }, now)).toEqual({ from: "2026-09-16" });
    expect(resolveActivityWindow({ range: "7d" }, now)).toEqual({ from: "2026-09-09" });
    const params = activityApiQuery({ range: "7d", model: "tokenhub/echo-1", result: "failed", billing: "confirmed" }, now);
    expect(params.get("limit")).toBe("100");
    expect(params.get("from")).toBe("2026-09-09");
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
