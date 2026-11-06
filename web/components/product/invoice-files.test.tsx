// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { InvoiceFiles } from "./invoice-files";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const file = { id: "file-1", filename: "dot.png", content_type: "image/png", size_bytes: 68, state: "ready", created_at: "2026-11-03T10:05:00Z", download_url: "/api/v1/operators/op/invoices/inv/files/file-1" };

it("removes a mistaken invoice file after confirmation and labels the upload plainly", async () => {
  let removed = false;
  const fetch = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "DELETE") { removed = true; return new Response(JSON.stringify({ id: "file-1", status: "removed" }), { status: 200 }); }
    return new Response(JSON.stringify({ items: removed ? [] : [file] }), { status: 200 });
  });
  vi.stubGlobal("fetch", fetch);
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><InvoiceFiles embedded scope="/api/v1/operators/op/invoices" invoiceID="inv" upload /></QueryClientProvider>);
  expect(await screen.findByText("dot.png")).toBeTruthy();
  expect(screen.getByRole("button", { name: "上传文件" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "上传真实私有文件" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "删除" }));
  fireEvent.click(screen.getByRole("button", { name: "确认删除" }));
  await waitFor(() => expect(fetch.mock.calls.some(([input, init]) => String(input) === "/api/v1/operators/op/invoices/inv/files/file-1" && init?.method === "DELETE")).toBe(true));
});

it("shows recipient details inside the invoice result form", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ items: [], details: { title: "Buyer Ltd", tax_id: "TAX-123", email: "billing@example.test", address: "12 Test Road" } }), { status: 200 })));
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><InvoiceFiles embedded scope="/api/v1/operators/op/invoices" invoiceID="inv" upload /></QueryClientProvider>);
  expect(await screen.findByText("Buyer Ltd")).toBeTruthy();
  for (const text of ["TAX-123", "billing@example.test", "12 Test Road"]) expect(screen.getByText(text)).toBeTruthy();
});
