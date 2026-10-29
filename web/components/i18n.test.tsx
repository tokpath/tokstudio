// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { I18nProvider, LanguageControl } from "./i18n";
import { Heading, Money, When } from "./product/shared";
import { request } from "@/lib/api-client";
import { CommandResultSchema } from "@/lib/contracts";
import { localeTags, translate } from "@/lib/i18n";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); document.cookie = "NEXT_LOCALE=; Max-Age=0; Path=/"; document.documentElement.lang = "zh-CN"; });
it("preserves a brand name that happens to match system copy", () => {
  render(<I18nProvider initialLocale="en"><Heading eyebrow="客户" title="模型与价格" /></I18nProvider>);
  expect(screen.getByText("客户")).toBeTruthy();
  expect(screen.getByRole("heading", { name: translate("en", "模型与价格") })).toBeTruthy();
});
it("persists language, preserves field identity, and sends it without changing action identity", async () => {
  const fetch = vi.fn(async () => new Response(JSON.stringify({ id: "same-operation", status: "pending" }), { status: 200 })); vi.stubGlobal("fetch", fetch);
  render(<I18nProvider><LanguageControl /><input aria-label="authored name" defaultValue="客户" /><Money value="12345.000000000000000001" unit="USD" /><When value="2026-10-18T12:34:00Z" /></I18nProvider>);
  const input = screen.getByRole("textbox"), date = new Date("2026-10-18T12:34:00Z");
  const original = { name: "客户", amount_money: "0.000000000000000001" };
  for (const locale of ["en", "ja", "ru", "zh"] as const) {
    fireEvent.change(screen.getByTestId("language-selector"), { target: { value: locale } });
    expect(document.documentElement.lang).toBe(localeTags[locale]); expect(document.cookie).toContain(`NEXT_LOCALE=${locale}`);
    expect(screen.getByRole("textbox")).toBe(input); expect((input as HTMLInputElement).value).toBe("客户");
    expect(screen.getByLabelText(translate(locale, "显示语言"))).toBeTruthy();
    expect(screen.getByLabelText("12345.000000000000000001 USD")).toBeTruthy();
    expect(document.querySelector("time")!.textContent).toBe(new Intl.DateTimeFormat(localeTags[locale], { dateStyle: "medium", timeStyle: "short" }).format(date));
    await request("/api/v1/me/withdrawals", CommandResultSchema, { method: "POST", actionKey: "same-key", body: original });
    const [, options] = fetch.mock.calls.at(-1)! as unknown as [string, RequestInit];
    expect((options.headers as Headers).get("Accept-Language")).toBe(locale);
    expect((options.headers as Headers).get("Idempotency-Key")).toBe("same-key"); expect(options.body).toBe(JSON.stringify(original));
  }
});
