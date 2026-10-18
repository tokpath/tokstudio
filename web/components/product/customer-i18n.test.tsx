// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { I18nProvider, useI18n } from "@/components/i18n";
import { locales, translate } from "@/lib/i18n";
import customerMessages from "@/locales/customer.json";
import { CustomerPage } from "./customer";
import { CustomerKeys } from "./customer-keys";
import { apiExample } from "./customer-models";
import { fundingPreview } from "./customer-finance";

vi.mock("./workspace-shell", () => ({ WorkspaceShell: ({ children }: { children: React.ReactNode }) => <div>{children}</div> }));
vi.mock("./notification-preferences", () => ({ NotificationPreferences: () => null }));

function LocaleControls() {
  const { setLocale } = useI18n();
  return <div>{locales.map(locale => <button key={locale} onClick={() => setLocale(locale)}>{locale}</button>)}</div>;
}

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it("switches a customer form and its validation through all four languages while retaining authored input", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ items: [] }), { status: 200 })));
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<I18nProvider><QueryClientProvider client={cache}><LocaleControls /><CustomerKeys /></QueryClientProvider></I18nProvider>);
  fireEvent.click(screen.getByRole("button", { name: "创建 Key", exact: true }));
  const nameInput = screen.getByLabelText("名称", { exact: true }) as HTMLInputElement;
  fireEvent.change(nameInput, { target: { value: "用户命名的生产应用" } });
  fireEvent.change(screen.getByLabelText("ATG 预算上限"), { target: { value: "0.000000000000000001" } });
  fireEvent.change(screen.getByLabelText("每分钟请求上限"), { target: { value: "1.5" } });
  fireEvent.click(screen.getByRole("button", { name: "创建 Key", exact: true }));
  const validation = "请检查预算金额；请求和并发上限须为正整数，留空表示不额外限制。";
  for (const locale of locales) {
    fireEvent.click(screen.getByRole("button", { name: locale, exact: true }));
    const currentName = screen.getByLabelText(translate(locale, "名称"), { exact: true }) as HTMLInputElement;
    expect(currentName).toBe(nameInput);
    expect(currentName.value).toBe("用户命名的生产应用");
    expect((screen.getByLabelText(translate(locale, "ATG 预算上限")) as HTMLInputElement).value).toBe("0.000000000000000001");
    expect(screen.getByRole("alert").textContent).toBe(translate(locale, validation));
    expect(screen.getByRole("button", { name: translate(locale, "创建 Key"), exact: true })).toBeTruthy();
  }
});

it("localizes generated example placeholders and finance validation without changing model identifiers", () => {
  for (const locale of locales) {
    const model = "原始模型-id";
    const example = apiExample("https://api.example.test/v1", model, "transcription", "curl", locale);
    expect(example).toContain(model);
    expect(example).toContain(translate(locale, "<本地音频路径>"));
    expect(example).toContain(translate(locale, "<为本次请求生成唯一编号>"));
    expect(fundingPreview("1", "2", "0", undefined, undefined, locale).error).toBe(translate(locale, "支付来源合计超过本次应付，请调整分配。"));
  }
});

it("provides complete translations with matching named placeholders for the customer catalog", () => {
  const placeholders = (value: string) => [...value.matchAll(/\{([A-Za-z_][A-Za-z0-9_]*)\}/g)].map(match => match[1]).sort();
  for (const [source, message] of Object.entries(customerMessages)) {
    for (const locale of ["en", "ja", "ru"] as const) {
      expect(message[locale].trim(), `${locale}: ${source}`).not.toBe("");
      expect(placeholders(message[locale]), `${locale}: ${source}`).toEqual(placeholders(source));
    }
  }
});

it("translates trusted system notifications and preserves authored broadcasts with identical catalog words", async () => {
  const items = [
    { id: "system-notice", title: "订单已建立", body: "服务额度已到账，可以开始使用。", category: "mandatory", necessary: true, system_generated: true, read_at: null, created_at: "2026-10-18T00:00:00Z" },
    { id: "authored-notice", title: "通知", body: "名称", category: "mandatory", necessary: true, system_generated: false, read_at: null, created_at: "2026-10-18T00:00:00Z" },
  ];
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ items }), { status: 200 })));
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<I18nProvider initialLocale="en"><QueryClientProvider client={cache}><LocaleControls /><CustomerPage section="notifications" /></QueryClientProvider></I18nProvider>);
  await screen.findByText(translate("en", "订单已建立"));
  for (const locale of ["en", "ja", "ru"] as const) {
    fireEvent.click(screen.getByRole("button", { name: locale, exact: true }));
    expect(screen.getByText(translate(locale, "订单已建立"))).toBeTruthy();
    expect(screen.getByText(translate(locale, "服务额度已到账，可以开始使用。"))).toBeTruthy();
    expect(screen.getByRole("heading", { level: 2, name: "通知", exact: true })).toBeTruthy();
    expect(screen.getByText("名称")).toBeTruthy();
  }
});
