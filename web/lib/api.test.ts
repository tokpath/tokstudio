import { describe, expect, it } from "vitest";
import { resolveBrowserApiBase, resolveServerApiBase } from "./api";

describe("resolveBrowserApiBase", () => {
  it("defaults to the same-origin /api prefix", () => {
    expect(resolveBrowserApiBase()).toBe("/api");
    expect(resolveBrowserApiBase("")).toBe("/api");
  });

  it("keeps an explicit absolute API origin", () => {
    expect(resolveBrowserApiBase("http://localhost:8080/")).toBe("http://localhost:8080");
  });
});

describe("resolveServerApiBase", () => {
  it("prefers the compose-internal URL", () => {
    expect(resolveServerApiBase("http://api:8080/", "http://localhost")).toBe("http://api:8080");
  });

  it("falls back to the public API origin", () => {
    expect(resolveServerApiBase(undefined, "http://127.0.0.1:8080/")).toBe("http://127.0.0.1:8080");
  });
});
