import { describe, expect, it } from "vitest";
import { authContinuation, safeCustomerNext } from "./customer-next";

describe("authentication continuation allowlist", () => {
  it.each([
    ["/workspace/models?model=public-model", "/workspace/models?model=public-model"],
    ["/workspace/plans?kol_activity=kol-one&plan=plan-one&invite=someone", "/workspace/plans?plan=plan-one&kol_activity=kol-one"],
    ["/workspace/playground?model=%E6%A8%A1%E5%9E%8B&next=https%3A%2F%2Fother.test", "/workspace/playground?model=%E6%A8%A1%E5%9E%8B"],
    ["/workspace/models?plan=unrelated", "/workspace/models"],
    ["/workspace/plans?plan=first&plan=second", "/workspace/plans?plan=first"],
    ["/workspace/models?model=%5C%5Cevil.test", "/workspace/models"],
  ])("preserves only the supported task and query: %s", (input, expected) => expect(safeCustomerNext(input)).toBe(expected));
  it.each([undefined, "", "https://evil.test/workspace/models", "//evil.test/workspace/plans", "/\\evil.test/workspace/models", "/workspace/models\\evil", "/workspace/plans/../models", "/workspace%2fmodels", "/workspace/models/", "/workspace/orders", "/manage/operator/plans", "/workspace/models#//evil", "/workspace/models?model=%ZZ", "/workspace/models?model=bad\nvalue"])("rejects unsafe or unsupported continuations: %s", input => expect(safeCustomerNext(input)).toBeUndefined());
  it("keeps the invitation separate from the requested task when switching auth screens", () => {
    const url = authContinuation("/register", "/workspace/plans?plan=plan-one&kol_activity=kol-one", "original-inviter");
    const parsed = new URL(url, "https://brand.test");
    expect(parsed.searchParams.get("next")).toBe("/workspace/plans?plan=plan-one&kol_activity=kol-one");
    expect(parsed.searchParams.get("invite")).toBe("original-inviter");
  });
});
