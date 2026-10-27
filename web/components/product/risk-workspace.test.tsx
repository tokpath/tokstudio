// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { ResourceCollection } from "./operator-resources";
import type { ResourcePage } from "@/lib/contracts";

vi.mock("./context", () => ({ useProduct: () => ({ session: { kind: "staff", staff: { id: "staff-one" }, workspaces: [{ operator: { id: "operator-one", kind: "oem" }, permissions: ["operations.manage", "risk.read", "alerts.read"] }] } }) }));
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it("edits the selected real rule with exact thresholds, recipient arrays and its current revision", async () => {
  const page: ResourcePage = { title: "预警规则", items: [{ id: "reserve-warning", version: 8, status: "enabled", values: { id: "reserve-warning", metric: "procurement_available_atg", warning: "10.000000000000000001", critical: "2.000000000000000001", direction: "below", recipients: ["owner-one", "owner-two"], escalation_recipients: ["finance-one"], reminder_seconds: 3600, escalate_seconds: 7200, enabled: true }, allowed_actions: ["revise_rule"] }], columns: [{ key: "id", label: "规则编号", kind: "text" }], operations: [], row_operations: [{ name: "revise_rule", label: "修订此预警规则", method: "POST", path: "/api/v1/operators/operator-one/alerts", permission: "operations.manage", fields: [{ name: "id", label: "规则编号", kind: "hidden", required: true }, { name: "metric", label: "指标", kind: "text", required: true }, { name: "warning", label: "预警阈值", kind: "decimal", required: true }, { name: "critical", label: "严重阈值", kind: "decimal", required: true }, { name: "direction", label: "触发方向", kind: "select", required: true, options: [{ value: "below", label: "低于阈值" }, { value: "above", label: "高于阈值" }] }, { name: "recipients", label: "通知负责人", kind: "csv", required: true }, { name: "escalation_recipients", label: "升级负责人", kind: "csv" }, { name: "reminder_seconds", label: "提醒间隔秒数", kind: "integer", required: true }, { name: "escalate_seconds", label: "升级等待秒数", kind: "integer", required: true }, { name: "enabled", label: "启用", kind: "boolean", required: true }, { name: "expected_version", label: "当前规则版本", kind: "hidden", default_value: 0 }] }] };
  const fetch = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(JSON.stringify({ id: "reserve-warning", status: "saved", version: 9 }), { status: 200 }));
  vi.stubGlobal("fetch", fetch);
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><ResourceCollection operatorID="operator-one" resource="alerts" page={page} /></QueryClientProvider>);
  fireEvent.click(screen.getByRole("button", { name: "修订此预警规则" }));
  const dialog = screen.getByRole("dialog");
  expect(within(dialog).queryByLabelText("当前规则版本")).toBeNull();
  expect((within(dialog).getByLabelText("通知负责人", { exact: false }) as HTMLInputElement).value).toBe("owner-one,owner-two");
  fireEvent.change(within(dialog).getByLabelText("预警阈值", { exact: false }), { target: { value: "11.000000000000000001" } });
  fireEvent.click(within(dialog).getByRole("button", { name: "修订此预警规则", exact: true }));
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
  expect(JSON.parse(String(fetch.mock.calls[0]?.[1]?.body))).toEqual({ id: "reserve-warning", metric: "procurement_available_atg", warning: "11.000000000000000001", critical: "2.000000000000000001", direction: "below", recipients: ["owner-one", "owner-two"], escalation_recipients: ["finance-one"], reminder_seconds: 3600, escalate_seconds: 7200, enabled: true, expected_version: 8 });
});
