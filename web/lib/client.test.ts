import { describe, expect, it } from "vitest";
import { hasGeneratedPath } from "./client";

describe("generated OpenAPI client", () => {
  it("includes P0 admin catalog and 2FA paths", () => {
    expect(hasGeneratedPath("GET", "/admin/providers")).toBe(true);
    expect(hasGeneratedPath("POST", "/admin/models")).toBe(true);
    expect(hasGeneratedPath("GET", "/admin/routes")).toBe(true);
    expect(hasGeneratedPath("POST", "/admin/me/2fa/setup")).toBe(true);
    expect(hasGeneratedPath("POST", "/v1/me/api-keys/{id}/rotate")).toBe(true);
    expect(hasGeneratedPath("POST", "/v1/me/api-keys/{id}/copy")).toBe(true);
    expect(hasGeneratedPath("GET", "/v1/public/tls-check")).toBe(true);
    expect(hasGeneratedPath("GET", "/admin/billing/export")).toBe(true);
    expect(hasGeneratedPath("GET", "/v1/me/media")).toBe(true);
    expect(hasGeneratedPath("GET", "/admin/media")).toBe(true);
    expect(hasGeneratedPath("GET", "/admin/ops/dashboard")).toBe(true);
  });
});
