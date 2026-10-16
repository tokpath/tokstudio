// @vitest-environment jsdom
import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { useCommand } from "./shared";
import { CommandResultSchema } from "@/lib/contracts";
afterEach(() => vi.unstubAllGlobals());
it("retries the same unknown operation with its exact body, then gives a new action a new key", async () => {
  const fetch = vi.fn().mockRejectedValueOnce(new TypeError("connection lost")).mockImplementation(async () => new Response(JSON.stringify({ id: "order-1", status: "pending" }), { status: 200 })); vi.stubGlobal("fetch", fetch);
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={cache}>{children}</QueryClientProvider>;
  const { result } = renderHook(() => useCommand("/api/v1/me/orders", CommandResultSchema), { wrapper });
  await act(async () => { await result.current.run({ amount_usd: "1.000000000000000001" }); }); expect(result.current.unresolved).toBe(true);
  await act(async () => { await result.current.run({ amount_usd: "999" }); });
  expect(fetch.mock.calls[0][1].headers.get("Idempotency-Key")).toBe(fetch.mock.calls[1][1].headers.get("Idempotency-Key")); expect(fetch.mock.calls[1][1].body).toBe(fetch.mock.calls[0][1].body);
  await act(async () => { await result.current.run({ amount_usd: "2" }); }); expect(fetch.mock.calls[2][1].headers.get("Idempotency-Key")).not.toBe(fetch.mock.calls[1][1].headers.get("Idempotency-Key"));
});
