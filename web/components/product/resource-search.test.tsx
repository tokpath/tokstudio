// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { ResourceCollection, ResourceScreen } from "./operator-resources";
import type { ResourcePage } from "@/lib/contracts";

vi.mock("./context", () => ({ useProduct: () => ({ session: { kind: "staff", workspaces: [{ operator: { id: "operator-search", kind: "channel" }, permissions: ["*"] }] } }) }));
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const result = (items: { id: string; values: Record<string, string> }[], next_cursor = "") => new Response(JSON.stringify({ title: "业务记录", description: "真实范围", items, columns: [{ key: "id", label: "编号", kind: "text" }, { key: "reference", label: "凭证", kind: "text" }], operations: [], row_operations: [], next_cursor }), { status: 200, headers: { "content-type": "application/json" } });
function show(resource: string) { const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } }); render(<QueryClientProvider client={cache}><ResourceScreen operatorID="operator-search" resource={resource} /></QueryClientProvider>); }

it("searches invoices on the server so records outside the loaded page can be found", async () => {
  const fetch = vi.fn(async (input: RequestInfo | URL) => result(String(input).includes("q=older-invoice") ? [{ id: "older-invoice", values: { reference: "original-proof" } }] : [{ id: "recent-invoice", values: {} }]));
  vi.stubGlobal("fetch", fetch); show("invoices");
  await screen.findByText("recent-invoice");
  fireEvent.change(screen.getByLabelText("搜索记录"), { target: { value: "older-invoice" } });
  fireEvent.click(screen.getByRole("button", { name: "查询", exact: true }));
  await screen.findByText("older-invoice");
  expect(screen.queryByText("recent-invoice")).toBeNull();
  expect(fetch.mock.calls.some(call => String(call[0]).endsWith("/invoices?q=older-invoice"))).toBe(true);
  expect(screen.getByText("在当前权限范围内检索记录，结果按当前列表规则分页。")).toBeTruthy();
});

it("labels an unsupported endpoint as a current-page filter and retains the cursor while filtering", async () => {
  const fetch = vi.fn(async (input: RequestInfo | URL) => String(input).includes("cursor=next-page") ? result([{ id: "payment-c", values: { reference: "Alpha bank page two" } }]) : result([{ id: "payment-a", values: { reference: "Alpha bank" } }, { id: "payment-b", values: { reference: "Beta bank" } }], "next-page"));
  vi.stubGlobal("fetch", fetch); show("payments");
  await screen.findByText("payment-a");
  fireEvent.change(screen.getByLabelText("搜索记录"), { target: { value: "ALPHA" } });
  fireEvent.click(screen.getByRole("button", { name: "筛选本页", exact: true }));
  expect(screen.queryByText("payment-b")).toBeNull(); expect(screen.getByText("本页匹配 1 / 2 条 · 还有后续记录")).toBeTruthy();
  expect(fetch).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "下一页", exact: true }));
  await screen.findByText("payment-c");
  expect(String(fetch.mock.calls[1][0])).toBe("/api/v1/operators/operator-search/payments?cursor=next-page");
  fireEvent.click(screen.getByRole("button", { name: "清除", exact: true }));
  await waitFor(() => expect(screen.getByText("payment-c")).toBeTruthy());
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(screen.getByText("仅筛选当前已载入页；其他页尚未检索，可翻页继续筛选。")).toBeTruthy();
});

it("labels refund record types in Chinese while filtering by their unchanged raw values", () => {
  const page: ResourcePage = { title: "退款办理", description: "原退款记录", items: [
    { id: "refund-case", values: { record_kind: "case" } },
    { id: "refund-leg", values: { record_kind: "leg" } },
    { id: "refund-payment", values: { record_kind: "actual_refund_payment" } },
    { id: "refund-extension", values: { record_kind: "future_record_kind" } },
  ], columns: [{ key: "id", label: "编号", kind: "text" }, { key: "record_kind", label: "记录类型", kind: "text" }], operations: [], row_operations: [] };
  render(<ResourceCollection operatorID="operator-search" resource="refunds" page={page} />);
  for (const label of ["退款事项", "退款分项", "实际退款付款", "future_record_kind"]) {
    expect(screen.getByRole("button", { name: label, exact: true })).toBeTruthy();
    expect(screen.getByRole("cell", { name: label, exact: true })).toBeTruthy();
  }
  fireEvent.click(screen.getByRole("button", { name: "实际退款付款", exact: true }));
  expect(screen.getByRole("cell", { name: "refund-payment", exact: true })).toBeTruthy();
  expect(screen.queryByRole("cell", { name: "refund-case", exact: true })).toBeNull();
  expect(page.items[2].values.record_kind).toBe("actual_refund_payment");
  fireEvent.click(screen.getByRole("button", { name: "future_record_kind", exact: true }));
  expect(screen.getByRole("cell", { name: "refund-extension", exact: true })).toBeTruthy();
});

it("matches visible Chinese refund type names and their original values in the current-page search", async () => {
  const page: ResourcePage = { title: "退款办理", description: "原退款记录", items: [
    { id: "refund-case", values: { record_kind: "case" } },
    { id: "refund-leg", values: { record_kind: "leg" } },
    { id: "refund-payment", values: { record_kind: "actual_refund_payment" } },
  ], columns: [{ key: "id", label: "编号", kind: "text" }, { key: "record_kind", label: "记录类型", kind: "text" }], operations: [], row_operations: [] };
  const fetch = vi.fn(async () => new Response(JSON.stringify(page), { status: 200, headers: { "content-type": "application/json" } }));
  vi.stubGlobal("fetch", fetch); show("refunds");
  await screen.findByRole("cell", { name: "refund-case", exact: true });
  for (const term of ["退款事项", "case"]) {
    fireEvent.change(screen.getByLabelText("搜索记录"), { target: { value: term } });
    fireEvent.click(screen.getByRole("button", { name: "筛选本页", exact: true }));
    expect(screen.getByRole("cell", { name: "refund-case", exact: true })).toBeTruthy();
    expect(screen.queryByRole("cell", { name: "refund-leg", exact: true })).toBeNull();
    expect(screen.queryByRole("cell", { name: "refund-payment", exact: true })).toBeNull();
  }
  fireEvent.change(screen.getByLabelText("搜索记录"), { target: { value: "actual_refund_payment" } });
  fireEvent.click(screen.getByRole("button", { name: "筛选本页", exact: true }));
  expect(screen.getByRole("cell", { name: "refund-payment", exact: true })).toBeTruthy();
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(page.items.map(item => item.values.record_kind)).toEqual(["case", "leg", "actual_refund_payment"]);
});
