import { describe, expect, it } from "vitest";
import { applyListResult, initialListSnapshot, isSessionLoss } from "./list-resource";

describe("list resource states", () => {
  it("keeps first load as loading until a result arrives", () => {
    expect(initialListSnapshot().phase).toBe("loading");
  });

  it("treats success with no rows as empty, not error", () => {
    const next = applyListResult(initialListSnapshot<string>(), { ok: true, status: 200, items: [] });
    expect(next.phase).toBe("empty");
    expect(next.items).toEqual([]);
  });

  it("does not turn a first-load failure into empty", () => {
    const next = applyListResult(initialListSnapshot<{ id: string }>(), {
      ok: false,
      status: 500,
      message: "upstream timeout",
    });
    expect(next.phase).toBe("error");
    expect(next.items).toEqual([]);
    expect(next.message).toBe("upstream timeout");
  });

  it("sends anonymous channel 403 未授权 to login, not empty", () => {
    expect(isSessionLoss(403, "permission_denied", "未授权")).toBe(true);
    const next = applyListResult(initialListSnapshot(), { ok: false, status: 403, code: "permission_denied", message: "未授权" });
    expect(next.phase).toBe("unauthorized");
  });

  it("keeps previous rows when a refresh fails", () => {
    const ready = applyListResult(initialListSnapshot<{ id: string }>(), {
      ok: true,
      status: 200,
      items: [{ id: "u1" }],
    });
    const stale = applyListResult(ready, { ok: false, status: 502, message: "bad gateway" });
    expect(stale.phase).toBe("stale");
    expect(stale.items).toEqual([{ id: "u1" }]);
    expect(stale.message).toBe("bad gateway");
  });

  it("treats 401 after loaded data as session loss, not a stale refresh", () => {
    const ready = applyListResult(initialListSnapshot<{ id: string }>(), {
      ok: true,
      status: 200,
      items: [{ id: "u1" }],
    });
    const next = applyListResult(ready, { ok: false, status: 401, code: "authentication_error", message: "未登录" });
    expect(next.phase).toBe("unauthorized");
    expect(next.auth).toBe("session");
    expect(next.items).toEqual([]);
  });

  it("treats true forbidden after loaded data as unauthorized, not stale", () => {
    const ready = applyListResult(initialListSnapshot<{ id: string }>(), {
      ok: true,
      status: 200,
      items: [{ id: "u1" }],
    });
    const next = applyListResult(ready, { ok: false, status: 403, code: "permission_denied", message: "权限不足" });
    expect(next.phase).toBe("unauthorized");
    expect(next.auth).toBe("forbidden");
    expect(next.items).toEqual([]);
  });
});
