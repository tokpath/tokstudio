import { describe, expect, it } from "vitest";
import { messagesFor, resolveLocale, translate } from "./i18n";

describe("i18n", () => {
  it("defaults to Chinese and reserves English and Japanese", () => {
    expect(resolveLocale("nope")).toBe("zh");
    expect(messagesFor("en").admin.overview).toBe("Overview");
    expect(messagesFor("ja").admin.overview).toBe("概要");
    expect(translate("zh", "admin.title")).toContain("平台管理");
    expect(messagesFor("zh").admin.plans).toBe("套餐审核");
    expect(messagesFor("zh").admin.prices).toBe("价格");
    expect(messagesFor("en").admin.commission).toBe("Commission");
  });
});
