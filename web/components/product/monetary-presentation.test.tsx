// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { I18nProvider } from "@/components/i18n";
import type { Operation, ResourcePage } from "@/lib/contracts";
import { MonetaryProvider } from "./monetary";
import { OperationForm } from "./operation-form";
import { ResourceCollection } from "./operator-resources";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
function show(children: React.ReactNode) {
  return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><I18nProvider><MonetaryProvider value={{ currency: "CNY", atg_per_unit: "10.25", status: "locked", version: 7 }}>{children}</MonetaryProvider></I18nProvider></QueryClientProvider>);
}
const page: ResourcePage = { title: "收款", description: "", columns: [{ key: "amount_money", label: "金额 结算币种", kind: "money" }], items: [{ id: "receipt", values: { name: "Original USD receipt", currency: "USD", amount_money: "3" } }], operations: [], row_operations: [] };

it("uses the global currency for settlement amounts even when a receipt has an original currency", () => {
  show(<ResourceCollection page={page} resource="payments" />);
  expect(screen.getByRole("columnheader", { name: "金额 CNY" })).toBeTruthy();
  expect(screen.getByLabelText("3 CNY")).toBeTruthy();
  expect(screen.queryByLabelText("3 USD")).toBeNull();
});

it("keeps supplier reference and actual costs in their independent currencies", () => {
  show(<ResourceCollection page={{ ...page, items: [{ id: "cost", values: { provider_name: "Supplier", model_name: "Model", reference_amount: "4", reference_currency: "USD", amount: "5", currency: "MYR" } }] }} resource="supplier-costs" />);
  expect(screen.getByLabelText("4 USD")).toBeTruthy();
  expect(screen.getByLabelText("5 MYR")).toBeTruthy();
  expect(screen.queryByLabelText("4 CNY")).toBeNull();
});

it("shows the frozen currency on operator order amounts", () => {
  show(<ResourceCollection page={{ ...page, items: [{ id: "order", values: { name: "Order", amount_money: "6", monetary_snapshot: { currency: "EUR", atg_per_unit: "2.5", version: 4 } } }] }} resource="orders" />);
  expect(screen.getByLabelText("6 EUR")).toBeTruthy();
  expect(screen.queryByLabelText("6 CNY")).toBeNull();
});

it("checks original receipt fees separately from verified settlement fees without converting either", () => {
  vi.stubGlobal("fetch", vi.fn());
  const operation: Operation = { name: "receipt", label: "登记人工核实收款", method: "POST", path: "/api/v1/operators/platform/payments", fields: [
    { name: "currency", label: "真实收款币种", kind: "text", default_value: "USD" },
    { name: "original_amount", label: "实际原币收款总额（扣己方手续费前）", kind: "decimal", default_value: "10" },
    { name: "original_fee_amount", label: "己方收款手续费（原币，可稍后核实）", kind: "decimal", default_value: "1" },
    { name: "settlement_amount", label: "已核实订单入账金额（结算币种）", kind: "decimal", default_value: "70" },
    { name: "fee_money", label: "已核实清算手续费（结算币种）", kind: "decimal" },
  ] };
  show(<OperationForm operation={operation} operatorID="platform" />);
  const original = screen.getByRole("region", { name: "原币收款金额核对" }), settlement = screen.getByRole("region", { name: "收款金额核对" });
  expect(within(original).getByLabelText("9 USD")).toBeTruthy();
  expect(within(settlement).getByLabelText("70 CNY")).toBeTruthy();
  expect(within(settlement).getByText("手续费待核实，净到账金额暂不确定。")).toBeTruthy();
  fireEvent.change(screen.getByLabelText("已核实清算手续费（CNY）", { exact: false }), { target: { value: "2" } });
  expect(within(settlement).getByLabelText("68 CNY")).toBeTruthy();
  expect(within(original).getByLabelText("9 USD")).toBeTruthy();
  fireEvent.change(screen.getByLabelText("己方收款手续费（原币，可稍后核实）", { exact: false }), { target: { value: "11" } });
  fireEvent.click(screen.getByRole("button", { name: "登记人工核实收款", exact: true }));
  expect(screen.getByRole("alert").textContent).toContain("己方收款手续费不能超过收款总额。");
  expect(fetch).not.toHaveBeenCalled();
});
