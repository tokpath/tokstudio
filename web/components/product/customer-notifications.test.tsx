// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { Notifications } from "./customer";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const notice = { id: "notice-one", title: "提现已付款", body: "9.5 USD 已打出。", category: "mandatory", necessary: true, system_generated: false, read_at: null as string | null, created_at: "2026-10-30T08:33:00Z" };

it("marks every unread customer notice from the notification page", async () => {
  let marked = false;
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === "POST") {
      marked = true;
      return new Response(JSON.stringify({ id: "customer-one", status: "updated" }), { status: 200 });
    }
    if (url.includes("unread-count")) return new Response(JSON.stringify({ count: marked ? 0 : 1 }), { status: 200 });
    if (url.includes("preferences")) return new Response(JSON.stringify({ marketing: false, usage_email: true }), { status: 200 });
    return new Response(JSON.stringify({ items: [{ ...notice, read_at: marked ? "2026-10-30T09:00:00Z" : null }], next_cursor: null }), { status: 200 });
  });
  vi.stubGlobal("fetch", fetch);
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={cache}><Notifications /></QueryClientProvider>);
  fireEvent.click(await screen.findByRole("button", { name: "全部标为已读" }));
  await waitFor(() => expect(fetch.mock.calls.some(([input, init]) => String(input) === "/api/v1/me/notifications/read-all" && init?.method === "POST")).toBe(true));
  await waitFor(() => expect(screen.queryByRole("button", { name: "全部标为已读" })).toBeNull());
  expect(screen.queryByRole("button", { name: "标为已读" })).toBeNull();
});
