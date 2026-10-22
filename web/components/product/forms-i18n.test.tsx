// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { I18nProvider, useI18n } from "@/components/i18n";
import { translate, type Locale } from "@/lib/i18n";
import { OperationForm } from "./operation-form";
import { ConfigurationEditor } from "./operation-config-editor";
import { ConfigurationError, prepareConfiguration, type ConfigNode } from "./operation-config";
import type { Operation } from "@/lib/contracts";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
function LocaleButtons() { const { setLocale } = useI18n(); return <>{(["zh", "en", "ja", "ru"] as Locale[]).map(locale => <button key={locale} onClick={() => setLocale(locale)}>{locale}</button>)}</>; }
function show(operation: Operation) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<I18nProvider><LocaleButtons /><QueryClientProvider client={client}><OperationForm operation={operation} /></QueryClientProvider></I18nProvider>);
}
it("changes form labels and existing validation in every locale without replacing entered data", () => {
  const fetch = vi.fn(); vi.stubGlobal("fetch", fetch);
  show({ name: "email", label: "更新密码", method: "POST", path: "/api/v1/me/email", fields: [{ name: "email", label: "邮箱", kind: "email", required: true }] });
  const input = screen.getByLabelText("邮箱", { exact: false }) as HTMLInputElement;
  fireEvent.change(input, { target: { value: "customer-authored-invalid-address" } });
  fireEvent.click(screen.getByRole("button", { name: "更新密码", exact: true }));
  for (const locale of ["en", "ja", "ru", "zh"] as Locale[]) {
    fireEvent.click(screen.getByRole("button", { name: locale, exact: true }));
    expect(screen.getByLabelText(translate(locale, "邮箱"), { exact: false })).toBe(input);
    expect(input.value).toBe("customer-authored-invalid-address");
    expect(screen.getByRole("alert").textContent).toContain(translate(locale, "请填写有效的{label}", { label: translate(locale, "邮箱") }));
  }
  expect(fetch).not.toHaveBeenCalled();
});
it("keeps an unresolved command and its exact retry across language switches", async () => {
  const fetch = vi.fn().mockRejectedValueOnce(new TypeError("connection lost")).mockResolvedValueOnce(new Response(JSON.stringify({ id: "saved", status: "saved" }), { status: 200 })); vi.stubGlobal("fetch", fetch);
  show({ name: "amount", label: "更新密码", method: "POST", path: "/api/v1/me/amount", fields: [{ name: "amount", label: "基础单价", kind: "decimal", required: true }] });
  const input = screen.getByLabelText("基础单价", { exact: false }) as HTMLInputElement;
  fireEvent.change(input, { target: { value: "0.000000000000000001" } });
  fireEvent.click(screen.getByRole("button", { name: "更新密码", exact: true }));
  await screen.findByText("本次办理结果待核实");
  fireEvent.click(screen.getByRole("button", { name: "ru", exact: true }));
  expect(input.disabled).toBe(true); expect(input.value).toBe("0.000000000000000001");
  fireEvent.click(screen.getByRole("button", { name: translate("ru", "重试本次操作"), exact: true }));
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
  expect(fetch.mock.calls[1][1].body).toBe(fetch.mock.calls[0][1].body);
  expect(fetch.mock.calls[1][1].headers.get("Idempotency-Key")).toBe(fetch.mock.calls[0][1].headers.get("Idempotency-Key"));
});
it("localizes structured configuration paths without translating schema keys or business values", () => {
  const schema: ConfigNode = { kind: "object", label: "模型价格", fields: { amount: { kind: "decimal", label: "基础单价", required: true } } };
  let failure: ConfigurationError | undefined;
  try { prepareConfiguration({ amount: "invalid" }, schema); } catch (error) { failure = error as ConfigurationError; }
  expect(failure?.fieldPath).toBe("amount");
  for (const locale of ["zh", "en", "ja", "ru"] as Locale[]) {
    expect(failure?.localized(locale)).toContain(translate(locale, "模型价格"));
    expect(failure?.localized(locale)).toContain(translate(locale, "基础单价"));
  }
  const change = vi.fn(); const value = JSON.stringify({ amount: "1.000000000000000000000001", extension: "客户原文" });
  render(<I18nProvider><LocaleButtons /><ConfigurationEditor schema={schema} value={value} disabled={false} change={change} /></I18nProvider>);
  const input = screen.getByLabelText("基础单价", { exact: false });
  fireEvent.click(screen.getByRole("button", { name: "ja", exact: true }));
  expect(screen.getByLabelText(translate("ja", "基础单价"), { exact: false })).toBe(input);
  expect((input as HTMLInputElement).value).toBe("1.000000000000000000000001");
  expect(screen.queryByLabelText(translate("ja", "完整技术配置 JSON"))).toBeNull();
  expect(change).not.toHaveBeenCalled();
});
it("preserves operator-authored record keys inside localized configuration errors", () => {
  const schema: ConfigNode = { kind: "record", label: "参数允许范围", item: { kind: "decimal", label: "用量上界", required: true } };
  let failure: ConfigurationError | undefined;
  try { prepareConfiguration({ "邮箱": "invalid" }, schema); } catch (error) { failure = error as ConfigurationError; }
  expect(failure?.fieldPath).toBe(".邮箱");
  expect(failure?.localized("en")).toContain(`${translate("en", "参数允许范围")} · 邮箱`);
  expect(failure?.localized("ru")).toContain(`${translate("ru", "参数允许范围")} · 邮箱`);
});
