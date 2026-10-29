import { describe, expect, it } from "vitest";
import { readFileSync, readdirSync } from "node:fs";
import ts from "typescript";
import { catalogs, localizeNumberText, messages, normalizeLocale, resolveLocale, translate } from "./i18n";

describe("four-language presentation contract", () => {
  it.each([
    ["ru-RU", "en-US,en;q=0.9", "ru"], ["ja", "ru;q=1", "ja"],
    ["invalid", "fr;q=1,ru-RU;q=0.8,en;q=0.3", "ru"], [undefined, "ja-JP;q=0.9,zh;q=0.2", "ja"],
    ["", "en;q=0,ru;q=NaN,ja;q=3", "zh"], ["", "ru;q=0.8,en;q=0.8", "ru"],
    ["zh-Hant", "en", "zh"], ["fr", "de,*", "zh"],
  ])("negotiates cookie %s and header %s as %s", (cookie, header, want) => {
    expect(resolveLocale(cookie, header)).toBe(want);
  });
  it("rejects unsupported or malformed cookie language names", () => {
    expect(normalizeLocale("ru; Path=/")).toBeUndefined();
    expect(normalizeLocale("JA_jp")).toBe("ja");
  });
  it("has complete locale entries and preserves every named/printf parameter", () => {
    const parameters = (text: string) => (text.match(/\{[A-Za-z_]\w*\}|%[sd]/g) || []).sort();
    for (const [catalog, entries] of Object.entries(catalogs)) {
      for (const [source, message] of Object.entries(entries)) {
        for (const locale of ["en", "ja", "ru"] as const) {
          const value = message[locale];
          expect(value?.trim(), `${catalog}/${source}/${locale}`).toBeTruthy();
          expect(parameters(value), `${catalog}/${source}/${locale} parameters`).toEqual(parameters(source));
          if (locale !== "ja") expect(value, `${catalog}/${source}/${locale} contains untranslated Chinese`).not.toMatch(/\p{Script=Han}/u);
        }
      }
    }
  });
  it("covers explicitly marked Chinese system copy in every component", () => {
    function inspect(directory: URL) {
      for (const file of readdirSync(directory, { withFileTypes: true })) {
        const path = new URL(file.name + (file.isDirectory() ? "/" : ""), directory);
        if (file.isDirectory()) { inspect(path); continue; }
        if (!/\.tsx?$/.test(file.name) || /\.test\./.test(file.name)) continue;
        const tree = ts.createSourceFile(path.pathname, readFileSync(path, "utf8"), ts.ScriptTarget.Latest, true, file.name.endsWith("tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS);
        function visit(node: ts.Node) {
          if (ts.isCallExpression(node) && ts.isIdentifier(node.expression) && node.expression.text === "t") {
            const source = node.arguments[0];
            if (source && (ts.isStringLiteral(source) || ts.isNoSubstitutionTemplateLiteral(source)) && /\p{Script=Han}/u.test(source.text)) expect(messages[source.text], `${file.name}: ${source.text}`).toBeDefined();
          }
          ts.forEachChild(node, visit);
        }
        visit(tree);
      }
    }
    inspect(new URL("../components/", import.meta.url));
  });
  it("keeps interpolated financial values and authored names literal", () => {
    expect(translate("ru", "订单 {id}", { id: "客户" })).toBe("Заказ 客户");
    expect(translate("en", "赠送 0.000000000000000001 ATG；组合资格以确认报价为准")).toBe("Bonus 0.000000000000000001 ATG; combination eligibility is subject to the confirmed quote");
    expect(translate("ja", "Заказ order-123")).toBe("注文 order-123");
    expect(translate("en", "未收录的客户自定义说明")).toBe("未收录的客户自定义说明");
    expect(translate("en", "网络 {network}", { network: "{evil}" })).toBe("网络 {evil}");
  });
  it("retranslates already-visible validation copy without rebuilding its form", () => {
    const source = "请填写{name}";
    if (messages[source]) expect(translate("ru", translate("ja", source, { name: "external-id" }))).toBe(translate("ru", source, { name: "external-id" }));
    expect(translate("ja", translate("ru", "邮箱或密码错误"))).toBe(translate("ja", "邮箱或密码错误"));
    const detailed = "无法核实{label}，请重新选择业务记录后重试。";
    expect(translate("ru", translate("en", detailed, { label: "example" }))).toBe(translate("ru", detailed, { label: "example" }));
    expect(translate("ru", "请填写有效的foo")).toBe(translate("ru", "请填写有效的{label}", { label: "foo" }));
    const readiness = "仍有未满足事项：品牌未开放；负责人尚未开通经营方";
    expect(translate("ja", translate("en", readiness))).toBe(translate("ja", readiness));
  });
  it("changes display separators without rounding financial strings", () => {
    expect(localizeNumberText("12,345.000000000000000001", "ru")).toBe("12\u00a0345,000000000000000001");
    expect(localizeNumberText("0.000000000000000001", "en")).toBe("0.000000000000000001");
  });
  it.each(["zh", "en", "ja", "ru"] as const)("shows the configured settlement currency only in marked system templates in %s", locale => {
    const amount = translate(locale, "金额 结算币种", undefined, "CNY");
    expect(amount).toContain("CNY");
    expect(amount).not.toMatch(/结算币种|settlement currency|決済通貨|валюта расчётов/);
    const authoredName = "结算币种 settlement currency USD";
    expect(translate(locale, "{provider} · {amount} 结算币种 · 原开票方 {issuer}", { provider: authoredName, amount: "1.000000000000000001", issuer: "供货方" }, "CNY")).toContain(authoredName + " · 1.000000000000000001 CNY");
    expect(translate(locale, "自定义结算币种 USD 别名", undefined, "CNY")).toBe("自定义结算币种 USD 别名");
    expect(translate(locale, "结算币种（ISO 代码）", undefined, "CNY")).toBe(translate(locale, "结算币种（ISO 代码）"));
    expect(translate(locale, "美元 / 每秒", undefined, "CNY")).toBe(translate(locale, "美元 / 每秒"));
  });
});
