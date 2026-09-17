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
    expect(pagePathWithSearch("/app/activity", "result=failed")).toBe("/app/activity?result=failed");
    expect(loginHref("/app/activity?result=failed")).toBe("/login?next=%2Fapp%2Factivity%3Fresult%3Dfailed");
  });
});
