import { describe, expect, it } from "vitest";
import { loginHref, pagePathWithSearch, safeNextPath } from "./login-next";

describe("safeNextPath", () => {
  it("keeps in-app paths and rejects open redirects", () => {
    expect(safeNextPath("/")).toBe("/");
    expect(safeNextPath("/app")).toBe("/app");
    expect(safeNextPath("//evil.example")).toBe("");
    expect(safeNextPath("https://evil.example")).toBe("");
    expect(loginHref("/")).toBe("/login?next=%2F");
    expect(loginHref("/enter")).toBe("/login?next=%2Fenter");
  });

  it("keeps list filters on the login return path", () => {
    expect(pagePathWithSearch("/app/activity", "status=failed")).toBe("/app/activity?status=failed");
    expect(loginHref("/app/activity?status=failed")).toBe("/login?next=%2Fapp%2Factivity%3Fstatus%3Dfailed");
  });
});
