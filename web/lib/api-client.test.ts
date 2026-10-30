import { afterEach, describe, expect, it, vi } from "vitest";
import { APIError, request, setCSRFToken, warmConnection } from "./api-client";
import { CommandResultSchema, CreditsSchema } from "./contracts";
afterEach(() => { vi.unstubAllGlobals(); setCSRFToken(); });
describe("validated business HTTP client", () => {
  it("sends cookies, CSRF and exact decimal strings with the caller's stable key", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: "job-1", status: "pending" }), { status: 200 })); vi.stubGlobal("fetch", fetch); setCSRFToken("csrf-test");
    await request("/api/v1/me/withdrawals", CommandResultSchema, { method: "POST", actionKey: "action-1", body: { amount_money: "0.000000000000000001" } });
    const [, options] = fetch.mock.calls[0]; expect(options.credentials).toBe("include"); expect(options.headers.get("X-CSRF-Token")).toBe("csrf-test"); expect(options.headers.get("Idempotency-Key")).toBe("action-1"); expect(JSON.parse(options.body).amount_money).toBe("0.000000000000000001");
  });
  it("refuses mutations without an operation key before network access", async () => { const fetch = vi.fn(); vi.stubGlobal("fetch", fetch); await expect(request("/api/v1/me/orders", CommandResultSchema, { method: "POST", body: {} })).rejects.toThrow("stable idempotency key"); expect(fetch).not.toHaveBeenCalled(); });
  it("keeps network unknowns and business rejection facts distinct", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("offline")));
    await expect(request("/api/v1/me/orders", CommandResultSchema, { method: "POST", actionKey: "a", body: {} })).rejects.toMatchObject({ code: "network_unknown", retryable: true });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { code: "insufficient_balance", message: "可用额度不足", request_id: "req-1", retryable: false, next_action: "购买套餐" } }), { status: 409 })));
    await expect(request("/api/v1/me/orders", CommandResultSchema, { method: "POST", actionKey: "a", body: {} })).rejects.toMatchObject({ code: "insufficient_balance", requestId: "req-1", retryable: false });
  });
  it("does not disguise malformed successful responses as a completed action", async () => { vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ customer_atg: { available: 0.1 } }), { status: 200 }))); await expect(request("/api/v1/me/credits", CreditsSchema)).rejects.toBeInstanceOf(APIError); });
  it("retries a read once after a dropped connection", async () => {
    const fetch = vi.fn().mockRejectedValueOnce(new TypeError("offline")).mockResolvedValueOnce(new Response(JSON.stringify({ id: "order-1", status: "pending" }), { status: 200 }));
    vi.stubGlobal("fetch", fetch);
    await expect(request("/api/v1/me/orders/order-1", CommandResultSchema)).resolves.toMatchObject({ id: "order-1" });
    expect(fetch).toHaveBeenCalledTimes(2);
  });
  it("opens a fresh connection when the tab has been idle", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response("ok"));
    vi.stubGlobal("fetch", fetch);
    warmConnection();
    await new Promise(resolve => setTimeout(resolve, 0));
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(String(fetch.mock.calls[0][0])).toBe("/api/healthz");
  });
});
