import { describe, expect, it } from "vitest";
import { loginHref, safeNextPath } from "./login-next";

describe("safeNextPath", () => {
  it("keeps in-app paths and rejects open redirects", () => {
    expect(safeNextPath("/")).toBe("/");
    expect(safeNextPath("/app")).toBe("/app");
    expect(safeNextPath("//evil.example")).toBe("");
    expect(safeNextPath("https://evil.example")).toBe("");
    expect(loginHref("/")).toBe("/login?next=%2F");
  });
});
