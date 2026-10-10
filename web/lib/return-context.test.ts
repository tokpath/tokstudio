import { describe, expect, it } from "vitest";
import { appendReturnContext, safeReturnHref } from "./return-context";

describe("management return context", () => {
  it("preserves list search and cursor through a detail link", () => {
    const list = "/admin/models?_admin_models_q=echo&_admin_models_cursor=model120";
    const detail = appendReturnContext("/admin/models/vendor%2Fmodel", list);
    expect(safeReturnHref(new URL(detail, "https://test.invalid").searchParams.get("return_to"), "/admin/models")).toBe(list);
  });
  it("rejects external and cross-console return targets", () => {
    for (const value of ["https://evil.test", "//evil.test", "/\\evil.test", "/%5cevil.test", "/channel/payments"]) expect(safeReturnHref(value, "/admin/models")).toBe("/admin/models");
  });
});
