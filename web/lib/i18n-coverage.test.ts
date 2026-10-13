import { describe, expect, it } from "vitest";
import { createTranslator } from "next-intl";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";
import zh from "@/messages/zh.json";
import en from "@/messages/en.json";
import ja from "@/messages/ja.json";
import { alertMessage, deliveryTitle, notificationCopy, runbookCopy } from "./system-copy";
import { providerStatusLabel, protocolOptions, routeStrategyOptions } from "./catalog-admin";
import { planPeriodLabels } from "./plan-display";
import { perMillionToPerToken } from "./token-price";

function flatten(value: object, prefix = ""): Record<string, string> {
  return Object.fromEntries(Object.entries(value).flatMap(([key, item]) => typeof item === "string" ? [[prefix + key, item]] : Object.entries(flatten(item, prefix + key + "."))));
}
const catalogs = { zh, en, ja };
const translator = (locale: keyof typeof catalogs) => createTranslator({ locale, messages: catalogs[locale] });

describe("internationalization coverage", () => {
  it("has matching keys and dynamic placeholders in every language", () => {
    const source = flatten(zh);
    for (const locale of ["en", "ja"] as const) {
      const translated = flatten(catalogs[locale]);
      expect(Object.keys(translated).sort()).toEqual(Object.keys(source).sort());
      for (const [key, value] of Object.entries(source)) {
        const placeholders = (text: string) => [...new Set(text.match(/\{\w+\}/g) ?? [])].sort();
        expect(placeholders(translated[key]), key).toEqual(placeholders(value));
      }
    }
  });

  it("never copies Chinese prose into English translations", () => {
    for (const [key, value] of Object.entries(flatten(en))) {
      if (["chrome.localeZh", "chrome.localeJa"].includes(key)) continue;
      expect(value, key).not.toMatch(/[\u4e00-\u9fff]/);
    }
  });

  it("formats new translations with all dynamic values without ICU errors", () => {
    for (const locale of ["zh", "en", "ja"] as const) {
      const errors: unknown[] = [];
      const t = createTranslator({ locale, messages: catalogs[locale], onError: error => errors.push(error) });
      for (const [key, value] of Object.entries(flatten({ consoleCopy: catalogs[locale].consoleCopy }))) {
        const values = Object.fromEntries((value.match(/\{\w+\}/g) ?? []).map(name => [name.slice(1, -1), "audit-value"]));
        expect(t(key, values), key).not.toBe(key);
      }
      expect(errors).toEqual([]);
    }
  });

  it("does not hardcode Chinese UI text in production page components", () => {
    const violations: string[] = [];
    function walk(directory: string) {
      for (const item of readdirSync(directory, { withFileTypes: true })) {
        const file = join(directory, item.name);
        if (item.isDirectory()) { walk(file); continue; }
        if (!file.endsWith(".tsx") || file.includes(".test.")) continue;
        const source = ts.createSourceFile(file, readFileSync(file, "utf8"), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
        function visit(node: ts.Node) {
          if ((ts.isStringLiteralLike(node) || ts.isJsxText(node) || ts.isTemplateHead(node) || ts.isTemplateMiddle(node) || ts.isTemplateTail(node)) && /[\u4e00-\u9fff]/.test(node.text)) {
            violations.push(`${file}:${source.getLineAndCharacterOfPosition(node.getStart()).line + 1} ${node.text}`);
          }
          ts.forEachChild(node, visit);
        }
        visit(source);
      }
    }
    walk("app"); walk("components");
    expect(violations).toEqual([]);
  });

  it("localizes options and validation while keeping protocol IDs intact", () => {
    const t = translator("en");
    expect(protocolOptions("openai", t).find(row => row.value === "openai")?.label).toBe("OpenAI compatible");
    expect(providerStatusLabel("active", t)).toBe("Eligible for routing");
    expect(routeStrategyOptions("priority", t)[0]).toEqual({ value: "priority", label: "Specified order" });
    expect(planPeriodLabels(t).monthly).toBe("Monthly");
    expect(() => perMillionToPerToken("0.000002", t)).toThrow(/per million tokens/);
  });

  it.each(["en", "ja"] as const)("translates historical notifications, deliveries and runbooks in %s", locale => {
    const t = translator(locale);
    const item = { kind: "alert_opened", title: "平台运营告警", body: "平台出现新的运营告警，请查看告警详情并处理。\n尚未记录备份恢复演练" };
    const displayed = notificationCopy(item, locale, t);
    expect(displayed.title).toBe(t("noticeEvents.alert_opened.title"));
    expect(displayed.body).toContain(t("systemAlerts.backup"));
    expect(displayed.body).not.toContain("尚未记录备份恢复演练");
    expect(deliveryTitle({ kind: "renewal_failed", title: "Plan renewal failed" }, locale, t)).toBe(t("noticeEvents.renewal_failed.title"));
    expect(runbookCopy({ id: "rb_backup", title: "备份演练", body: "原文" }, locale, t).body).toContain("scripts/backup_drill.sh");
    expect(alertMessage("成功率低于阈值：客户自定义供应商", t)).toContain("客户自定义供应商");
  });

  it("preserves operator-authored and unknown notification content", () => {
    const item = { kind: "custom", title: "客户原始名称", body: "客户自行填写的说明" };
    expect(notificationCopy(item, "en", translator("en"))).toEqual(item);
    expect(notificationCopy({ ...item, kind: "renewal_failed" }, "zh", translator("zh"))).toEqual({ ...item, kind: "renewal_failed" });
  });
});
