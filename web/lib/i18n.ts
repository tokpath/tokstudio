import discoveryUx from "@/locales/discovery-ux.json";
import operatorUx from "@/locales/operator-ux.json";
import customerUx from "@/locales/customer-ux.json";
import help from "@/locales/help.json";
import common from "@/locales/common.json";
import customer from "@/locales/customer.json";
import operator from "@/locales/operator.json";
import forms from "@/locales/forms.json";
import backend from "@/locales/backend.json";
import errors from "@/locales/errors.json";

export const locales = ["zh", "en", "ja", "ru"] as const;
export type Locale = (typeof locales)[number];
export const localeTags: Record<Locale, string> = { zh: "zh-CN", en: "en", ja: "ja", ru: "ru" };
export const localeNames: Record<Locale, string> = { zh: "中文", en: "English", ja: "日本語", ru: "Русский" };
export const localeCookie = "NEXT_LOCALE";
export type TranslationValues = Record<string, string | number>;
export type Translator = (source: string, values?: TranslationValues) => string;
export type Message = { en: string; ja: string; ru: string; zh?: string };
export const catalogs = { common, customer, operator, forms, backend, errors, help, customerUx, operatorUx, discoveryUx };
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
const reverseMessages = new Map<string, Message>();
const templates: { expression: RegExp; keys: string[]; message: Message; specificity: number }[] = [];
const parameter = /\{([A-Za-z_][A-Za-z0-9_]*)\}|%[sd]/g;
const escapeExpression = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
for (const [source, message] of Object.entries(messages)) {
  const complete = { ...message, zh: message.zh || source };
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

export function translate(locale: Locale, source: string, values?: TranslationValues): string {
  const entry = reverseMessages.get(source);
  if (entry) return values ? interpolate(entry[locale] || source, values) : entry[locale] || source;
  if (!values && source.length <= 20000) {
    const requirement = messages["仍有未满足事项："];
    for (const prefix of ["仍有未满足事项：", requirement.en, requirement.ja, requirement.ru]) {
      if (source.startsWith(prefix)) {
        const items = source.slice(prefix.length).split(/；|\n/);
        return translate(locale, "仍有未满足事项：") + items.map(item => translate(locale, item)).join("\n");
      }
    }
    for (const template of templates) {
      const match = source.match(template.expression);
      if (match) return interpolate(template.message[locale] || source, Object.fromEntries(template.keys.map((key, index) => [key, match[index + 1]])));
    }
    // This suffix is appended to several trusted notification templates.
    const amount = source.match(/^(.*) 本次申请金额 ([\d.]+) USD。$/s);
    if (amount) return translate(locale, amount[1]) + translate(locale, " 本次申请金额 {amount} USD。", { amount: amount[2] });
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
