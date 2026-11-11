// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { I18nProvider } from "@/components/i18n";
import { translate, type Locale } from "@/lib/i18n";
import type { ResourcePage } from "@/lib/contracts";
import { ResourceCollection } from "./operator-resources";

vi.mock("./context", () => ({ useProduct: () => ({ session: { kind: "staff", staff: { id: "owner", display_name: "Owner", email: "owner@example.test" }, workspaces: [{ operator: { id: "platform", name: "Platform", kind: "platform", brand_id: "brand" }, permissions: ["*"] }] } }) }));
afterEach(() => { cleanup(); document.cookie = "NEXT_LOCALE=; Max-Age=0; Path=/"; });
const page: ResourcePage = {
  title: "后台任务异常", description: "", columns: [], operations: [], search_supported: false, status_supported: false,
  items: [
    { id: "failure", status: "dead", allowed_actions: ["redrive"], presentation: { title: "过期临时数据清理", labels: {} }, values: { name: "过期临时数据清理", impact_name: "Example OEM", failure_reason: "相关服务或业务状态异常，请由技术人员排查。", last_failed_at: "2026-11-11T04:00:00Z" } },
    { id: "retry", status: "pending", allowed_actions: [], values: { name: "发票状态检查", impact_scope: "全系统", failure_reason: "后台执行超时或中断，请检查任务服务。", last_failed_at: "2026-11-11T03:00:00Z", recovery_note: "正在自动重试" } },
  ],
  row_operations: [{ name: "redrive", label: "恢复任务", method: "POST", path: "/v1/operators/platform/jobs/{id}/redrive", fields: [], permission: "platform.manage", confirmation: "确认已排除故障，继续执行原任务。系统保留原业务编号，避免重复处理。" }],
};
function show(locale: Locale, value = page) {
  return render(<I18nProvider initialLocale={locale}><QueryClientProvider client={new QueryClient()}><ResourceCollection operatorID="platform" resource="jobs" page={value} /></QueryClientProvider></I18nProvider>);
}
it.each<Locale>(["zh", "en", "ja", "ru", "hi", "ms", "th"])("shows readable current failures and recovery without raw records in %s", locale => {
  show(locale);
  expect(screen.getByRole("heading", { name: translate(locale, "后台任务异常") })).toBeTruthy();
  expect(screen.getByText(translate(locale, "过期临时数据清理"))).toBeTruthy();
  expect(screen.getByText("Example OEM")).toBeTruthy();
  expect(screen.getByText(translate(locale, "全系统"))).toBeTruthy();
  expect(screen.getByText(translate(locale, "正在自动重试"))).toBeTruthy();
  expect(screen.getAllByRole("columnheader")).toHaveLength(5);
  expect(screen.queryByRole("button", { name: translate(locale, "查看详情") })).toBeNull();
  expect(screen.getAllByRole("button", { name: translate(locale, "恢复任务") })).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: translate(locale, "恢复任务") }));
  const panel = screen.getByRole("dialog");
  expect(within(panel).queryAllByRole("textbox")).toHaveLength(0);
  expect(within(panel).queryAllByRole("checkbox")).toHaveLength(0);
  expect(within(panel).queryByRole("list")).toBeNull();
  expect(within(panel).getByText(translate(locale, page.row_operations[0].confirmation!))).toBeTruthy();
});
it("explains the empty state without suggesting a new task or a business identifier", () => {
  show("zh", { ...page, items: [] });
  expect(screen.getByText("当前没有后台任务异常")).toBeTruthy();
  expect(screen.getByText("需要人工关注的失败任务会显示在这里，恢复成功后自动移除。")).toBeTruthy();
  expect(screen.queryByRole("table")).toBeNull();
});
