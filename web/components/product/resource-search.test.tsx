// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { ResourceCollection, ResourceScreen } from "./operator-resources";
import type { ResourcePage } from "@/lib/contracts";

vi.mock("./context", () => ({ useProduct: () => ({ session: { kind: "staff", staff: { id: "staff-search" }, workspaces: [{ operator: { id: "operator-search", kind: "channel" }, permissions: ["*"] }] } }) }));
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const result = (items: { id: string; values: Record<string, string> }[], next_cursor = "", search_supported = true) => new Response(JSON.stringify({ search_supported, status_supported: search_supported, title: "业务记录", description: "真实范围", items: items.map(item => ({...item,presentation:{title:item.id,labels:{}}})), columns: [{ key: "id", label: "编号", kind: "text" }, { key: "reference", label: "凭证", kind: "text" }], operations: [], row_operations: [], next_cursor }), { status: 200, headers: { "content-type": "application/json" } });
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

it("does not advertise filtering for a source that has no complete-scope search", async () => {
  const fetch = vi.fn(async (input: RequestInfo | URL) => String(input).includes("cursor=next-page") ? result([{ id: "evidence-c", values: { reference: "Alpha page two" } }], "", false) : result([{ id: "evidence-a", values: {} }], "next-page", false));
  vi.stubGlobal("fetch", fetch); show("evidence");
  await screen.findByText("evidence-a");
  expect(screen.queryByRole("textbox", { name: "搜索记录" })).toBeNull();
  expect(screen.queryByRole("combobox", { name: "筛选状态" })).toBeNull();
  expect(screen.queryByText("在当前权限范围内检索记录，结果按当前列表规则分页。")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "下一页", exact: true }));
  await screen.findByText("evidence-c");
  expect(String(fetch.mock.calls[1][0])).toBe("/api/v1/operators/operator-search/evidence?cursor=next-page");
});

it("groups Chinese refund types by their exact case and leg IDs and keeps future record types", () => {
  const page: ResourcePage = { title: "退款办理", description: "原退款记录", items: [
    { id: "refund-case", values: { record_kind: "case" } },
    { id: "refund-leg", values: { record_kind: "leg", case_id: "refund-case" } },
    { id: "refund-payment", values: { record_kind: "actual_refund_payment", leg_id: "refund-leg" } },
    { id: "refund-extension", values: { record_kind: "future_record_kind" } },
  ], columns: [{ key: "id", label: "编号", kind: "text" }, { key: "record_kind", label: "记录类型", kind: "text" }], operations: [], row_operations: [] };
  render(<ResourceCollection operatorID="operator-search" resource="refunds" page={page} record="refund-case" />);
  for (const label of ["退款事项", "退款分项", "实际退款付款", "future_record_kind"]) expect(screen.getByText(label, { exact: true })).toBeTruthy();
  const refund = screen.getByText("退款事项", {exact:true}).closest("section")!;
  expect(refund.textContent).toContain("实际退款付款");
  expect(refund.textContent).not.toContain("future_record_kind");
  expect(screen.getByText("本页其他关联记录")).toBeTruthy();
  expect(page.items[2].values.record_kind).toBe("actual_refund_payment");
});

it("shows one search on the provider page and keeps add actions on each section", () => {
  const page: ResourcePage = { search_supported: true, title: "模型供应商", description: "", items: [], columns: [], operations: [
    { name: "credential", label: "添加供应商密钥", method: "POST", path: "/api/v1/operators/operator-search/providers", fields: [] },
    { name: "model", label: "添加供应商模型", method: "POST", path: "/api/v1/operators/operator-search/providers", fields: [] },
  ], row_operations: [] };
  render(<ResourceCollection operatorID="operator-search" resource="providers" page={page} toolbar={<input aria-label="搜索记录" />} />);
  expect(screen.getByRole("heading", { name: "供应商密钥" })).toBeTruthy();
  expect(screen.getAllByRole("textbox", { name: "搜索记录" })).toHaveLength(1);
  expect(screen.getAllByRole("button", { name: "添加供应商密钥", exact: true })).toHaveLength(1);
  expect(screen.getAllByRole("button", { name: "添加供应商模型", exact: true })).toHaveLength(1);
});

it("forwards refund queries to the server and preserves its complete case context", async () => {
  const page: ResourcePage = { search_supported: true, status_supported: true, title: "退款办理", description: "原退款记录", items: [
    { id: "refund-case", values: { record_kind: "case" } },
    { id: "refund-leg", values: { record_kind: "leg", case_id: "refund-case" } },
    { id: "refund-payment", values: { record_kind: "actual_refund_payment", leg_id: "refund-leg" } },
  ], columns: [{ key: "id", label: "编号", kind: "text" }], operations: [], row_operations: [] };
  const fetch = vi.fn(async () => new Response(JSON.stringify(page), { status: 200, headers: { "content-type": "application/json" } }));
  vi.stubGlobal("fetch", fetch); show("refunds");
  await screen.findByText("退款事项", {exact:true});
  fireEvent.change(screen.getByLabelText("搜索记录"), { target: { value: "original-bank-proof" } });
  fireEvent.click(screen.getByRole("button", { name: "查询", exact: true }));
  await waitFor(() => expect(String(fetch.mock.calls.at(-1)?.[0])).toContain("q=original-bank-proof"));
  await screen.findByText("退款事项", {exact:true});
  expect(screen.getByText("退款分项", {exact:true})).toBeTruthy(); expect(screen.getByText("实际退款付款", {exact:true})).toBeTruthy();
  expect(page.items.map(item => item.values.record_kind)).toEqual(["case", "leg", "actual_refund_payment"]);
});
