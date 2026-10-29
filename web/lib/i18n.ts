import discoveryUx from "@/locales/discovery-ux.json";
import usabilityUx from "@/locales/usability-ux.json";
import operatorUx from "@/locales/operator-ux.json";
import customerUx from "@/locales/customer-ux.json";
import help from "@/locales/help.json";
import common from "@/locales/common.json";
import customer from "@/locales/customer.json";
import operator from "@/locales/operator.json";
import forms from "@/locales/forms.json";
import backend from "@/locales/backend.json";
import errors from "@/locales/errors.json";
import monetary from "@/locales/monetary.json";

export const locales = ["zh", "en", "ja", "ru"] as const;
export type Locale = (typeof locales)[number];
export const localeTags: Record<Locale, string> = { zh: "zh-CN", en: "en", ja: "ja", ru: "ru" };
export const localeNames: Record<Locale, string> = { zh: "中文", en: "English", ja: "日本語", ru: "Русский" };
export const localeCookie = "NEXT_LOCALE";
export type TranslationValues = Record<string, string | number>;
export type Translator = (source: string, values?: TranslationValues) => string;
export type Message = { en: string; ja: string; ru: string; zh?: string };
export const catalogs = { common, customer, operator, forms, backend, errors, help, customerUx, operatorUx, discoveryUx, usabilityUx, monetary };
export const messages: Record<string, Message> = Object.assign({}, ...Object.values(catalogs));

export function normalizeLocale(value?: string | null): Locale | undefined {
  const language = value?.trim().toLowerCase().split(/[-_]/)[0];
  return locales.find(locale => locale === language);
}

export function resolveLocale(cookie?: string | null, acceptLanguage = ""): Locale {
  const explicit = normalizeLocale(cookie);
  if (explicit) return explicit;
  const preferences = acceptLanguage.split(",").map((item, index) => {
    const [tag, ...parameters] = item.trim().split(";");
    const quality = parameters.map(value => value.trim()).find(value => value.startsWith("q="));
    const q = quality === undefined ? 1 : Number(quality.slice(2) || "NaN");
    return { locale: normalizeLocale(tag), q, index };
  }).filter(item => item.locale && Number.isFinite(item.q) && item.q > 0 && item.q <= 1);
  preferences.sort((a, b) => b.q - a.q || a.index - b.index);
  return preferences[0]?.locale || "zh";
}

// Only explicitly marked system copy is translated. Entity names, prompts,
// model identifiers and customer-authored content never pass through this API.
// Reverse lookup lets an existing validation/error state change language
// without resetting the form or an unresolved financial operation.
type SystemMessage = Message & { currencyAware: boolean };
const reverseMessages = new Map<string, SystemMessage>();
const templates: { expression: RegExp; keys: string[]; message: SystemMessage; specificity: number }[] = [];
// These name the setting itself, rather than an amount denominated in it.
const currencyConcepts = new Set([
  "全系统结算币种与 ATG 基础比例", "全系统使用同一结算币种。每 1 {currency} 对应 {ratio} ATG。",
  "结算币种（ISO 代码）", "每 1 单位结算币种购买的 ATG",
  "营业前请选择全系统的结算币种和 ATG 基础比例。",
]);
const parameter = /\{([A-Za-z_][A-Za-z0-9_]*)\}|%[sd]/g;
const escapeExpression = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
for (const [source, message] of Object.entries(messages)) {
  const complete = { ...message, zh: message.zh || source, currencyAware: source.includes("结算币种") && !currencyConcepts.has(source) };
  reverseMessages.set(source, complete);
  for (const locale of locales) {
    const text = complete[locale];
    if (text && !reverseMessages.has(text)) reverseMessages.set(text, complete);
    // Match only catalogued system sentences. Captured names, amounts and IDs
    // are interpolated verbatim, never passed back through the dictionary.
    if (text && /\{[A-Za-z_]\w*\}|%[sd]/.test(text)) {
      const keys: string[] = [];
      let end = 0, pattern = "^", ordinal = 0;
      for (const match of text.matchAll(parameter)) {
        pattern += escapeExpression(text.slice(end, match.index)) + "([\\s\\S]+?)";
        keys.push(match[1] || `printf${ordinal++}`);
        end = match.index! + match[0].length;
      }
      pattern += escapeExpression(text.slice(end)) + "$";
      // A bare parameter would turn arbitrary authored text into system copy.
      const specificity = text.replace(parameter, "").trim().length;
      if (specificity >= 2) templates.push({ expression: new RegExp(pattern), keys, message: complete, specificity });
    }
  }
}
// A short range such as "{min} to {max}" must never swallow a detailed
// validation sentence containing "to". Prefer the most specific known text.
templates.sort((a, b) => b.specificity - a.specificity);

function interpolate(text: string, values: TranslationValues): string {
  let ordinal = 0;
  return text.replace(parameter, (placeholder, named: string | undefined) => {
    const key = named || `printf${ordinal++}`;
    return Object.prototype.hasOwnProperty.call(values, key) ? String(values[key]) : placeholder;
  });
}

function systemText(message: SystemMessage, locale: Locale, source: string, settlementCurrency?: string): string {
  const text = message[locale] || source;
  if (!message.currencyAware || !settlementCurrency) return text;
  // Replace the catalog template before interpolation, preserving names and
  // explicit original currencies supplied as values or authored text.
  return text.replace(/结算币种|settlement currency|決済通貨|валют(?:а|ы|е|у|ой) расч[её]тов/gi, settlementCurrency);
}

export function translate(locale: Locale, source: string, values?: TranslationValues, settlementCurrency?: string): string {
  const entry = reverseMessages.get(source);
  if (entry) { const text = systemText(entry, locale, source, settlementCurrency); return values ? interpolate(text, values) : text; }
  if (!values && source.length <= 20000) {
    const requirement = messages["仍有未满足事项："];
    for (const prefix of ["仍有未满足事项：", requirement.en, requirement.ja, requirement.ru]) {
      if (source.startsWith(prefix)) {
        const items = source.slice(prefix.length).split(/；|\n/);
        return translate(locale, "仍有未满足事项：", undefined, settlementCurrency) + items.map(item => translate(locale, item, undefined, settlementCurrency)).join("\n");
      }
    }
    for (const template of templates) {
      const match = source.match(template.expression);
      if (match) return interpolate(systemText(template.message, locale, source, settlementCurrency), Object.fromEntries(template.keys.map((key, index) => [key, match[index + 1]])));
    }
    // This suffix is appended to several trusted notification templates.
    const amount = source.match(/^(.*) 本次申请金额 ([\d.]+) ([A-Z]{3}|结算币种)。$/s);
    if (amount) return translate(locale, amount[1], undefined, settlementCurrency) + translate(locale, " 本次申请金额 {amount} {currency}。", { amount: amount[2], currency: amount[3] === "结算币种" ? settlementCurrency || amount[3] : amount[3] });
  }
  return values ? interpolate(source, values) : source;
}

export function getClientLocale(): Locale {
  if (typeof document === "undefined") return "zh";
  const cookie = document.cookie.split(";").map(part => part.trim()).find(part => part.startsWith(`${localeCookie}=`));
  return normalizeLocale(cookie?.slice(localeCookie.length + 1)) || normalizeLocale(document.documentElement.lang) || "zh";
}

// Change separators only: do not convert high-precision financial strings to
// floating point or change the original value copied/submitted to the API.
export function localizeNumberText(value: string, locale: Locale): string {
  if (locale !== "ru") return value;
  return value.replace(/[,\.]/g, token => token === "," ? "\u00a0" : ",");
}
