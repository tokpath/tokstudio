import { afterEach, expect, it, vi } from "vitest";
import { generatedFetch } from "./generated/api";
import { HttpResponseError } from "./http-response";

afterEach(() => vi.unstubAllGlobals());

it.each([
  [503, "service_unavailable", "customer service unavailable"],
  [401, "authentication_error", "请重新登录"],
  [403, "permission_denied", "权限不足"],
])("rejects HTTP %i while preserving status, code and the original error body", async (status, code, message) => {
  const body = { error: { code, message }, request_id: "original-request" };
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json(body, { status })));
  const error = await generatedFetch("/api", "GET", "/channel/customers?q=retained").catch(value => value);
  expect(error).toBeInstanceOf(HttpResponseError);
  expect(error).toMatchObject({ status, code, message, body });
});

it("keeps the original successful typed body and request context", async () => {
  const body = { items: [{ id: "customer-1" }], total: 250, next_cursor: "page-2" };
  const fetcher = vi.fn().mockResolvedValue(Response.json(body));
  vi.stubGlobal("fetch", fetcher);
  const result = await generatedFetch<typeof body>("/api", "GET", "/channel/customers?q=retained&cursor=page-2", { credentials: "include", headers: { "Idempotency-Key": "original-operation" } });
  expect(result).toEqual(body);
  expect(fetcher).toHaveBeenCalledWith("/api/channel/customers?q=retained&cursor=page-2", { method: "GET", cache: "no-store", credentials: "include", headers: { "Idempotency-Key": "original-operation" } });
});

it("preserves the HTTP status even when a proxy returns a non-JSON error", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("unavailable", { status: 502 })));
  await expect(generatedFetch("/api", "GET", "/channel/customers")).rejects.toMatchObject({ status: 502, message: "HTTP 502" });
});
