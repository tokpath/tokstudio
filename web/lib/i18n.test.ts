import { describe, expect, it } from "vitest";
import { messagesFor, parseAcceptLanguage, resolveLocale, resolveRequestLocale, translate } from "./i18n";

describe("i18n", () => {
  it("defaults to Chinese and reserves English and Japanese", () => {
    expect(resolveLocale("nope")).toBe("zh");
    expect(messagesFor("en").admin.overview).toBe("Overview");
    expect(messagesFor("ja").admin.overview).toBe("概要");
    expect(translate("zh", "admin.title")).toContain("平台管理");
    expect(messagesFor("zh").admin.plans).toBe("套餐审核");
    expect(messagesFor("zh").admin.prices).toBe("价格");
    expect(messagesFor("zh").admin.promos).toBe("推广码");
    expect(messagesFor("en").admin.commission).toBe("Commission");
  });

  it("parses Accept-Language by quality and prefix", () => {
    expect(parseAcceptLanguage(null)).toBeNull();
    expect(parseAcceptLanguage("")).toBeNull();
    expect(parseAcceptLanguage("en-US,en;q=0.9")).toBe("en");
    expect(parseAcceptLanguage("ja-JP,ja;q=0.9,en;q=0.8")).toBe("ja");
    expect(parseAcceptLanguage("zh-CN,zh;q=0.9")).toBe("zh");
    expect(parseAcceptLanguage("fr-FR,fr;q=0.9")).toBeNull();
    expect(parseAcceptLanguage("fr;q=0.9,en;q=0.8,zh;q=0.1")).toBe("en");
    expect(parseAcceptLanguage("zh-TW;q=0.4,en-US;q=0.8")).toBe("en");
  });

  it("lets an explicit cookie override Accept-Language", () => {
    expect(resolveRequestLocale(null, "en-US")).toBe("en");
    expect(resolveRequestLocale(undefined, "ja")).toBe("ja");
    expect(resolveRequestLocale("ja", "en-US,en;q=0.9")).toBe("ja");
    expect(resolveRequestLocale("en", "zh-CN")).toBe("en");
    expect(resolveRequestLocale("nope", "en-US")).toBe("en");
    expect(resolveRequestLocale(null, null)).toBe("zh");
    expect(resolveRequestLocale("", "fr")).toBe("zh");
  });
});
