export const locales = ["zh", "en", "ja", "ru", "hi", "ms", "th"] as const;
export type Locale = (typeof locales)[number];
export const localeTags: Record<Locale, string> = { zh: "zh-CN", en: "en", ja: "ja", ru: "ru", hi: "hi", ms: "ms", th: "th" };
export const localeNames: Record<Locale, string> = { zh: "中文", en: "English", ja: "日本語", ru: "Русский", hi: "हिन्दी", ms: "Bahasa Melayu", th: "ไทย" };
export const localeCookie = "NEXT_LOCALE";
export type TranslationValues = Record<string, string | number>;
export type Translator = (source: string, values?: TranslationValues) => string;
export type Message = Record<Exclude<Locale, "zh">, string> & { zh?: string };
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
export type LocaleDictionary = Readonly<Record<string, string>>;
const dictionaries = new Map<Locale, LocaleDictionary>();
const sources = new Set<string>();
const reverseMessages = new Map<string, string>();
const indexedTemplates = new Set<string>();
const templates: { expression: RegExp; keys: string[]; source: string; specificity: number }[] = [];
// These name the setting itself, rather than an amount denominated in it.
const currencyConcepts = new Set([
  "全系统结算币种与 ATG 基础比例", "全系统使用同一结算币种。每 1 {currency} 对应 {ratio} ATG。",
  "结算币种（ISO 代码）", "每 1 单位结算币种购买的 ATG",
  "营业前请选择全系统的结算币种和 ATG 基础比例。",
]);
const parameter = /\{([A-Za-z_][A-Za-z0-9_]*)\}|%[sd]/g;
const escapeExpression = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
export function hasLocaleDictionary(locale: Locale): boolean {
  return dictionaries.has(locale);
}

// A locale dictionary contains static system text only. Keeping previous
// dictionaries lets errors already on screen change language in place.
export function installLocaleDictionary(locale: Locale, dictionary: LocaleDictionary) {
  if (dictionaries.has(locale)) return;
  dictionaries.set(locale, dictionary);
  for (const [source, localized] of Object.entries(dictionary)) {
    sources.add(source);
    if (!reverseMessages.has(localized)) reverseMessages.set(localized, source);
    indexTemplate(source, source);
    if (localized !== source) indexTemplate(localized, source);
  }
  // Prefer a specific validation sentence to a short range such as "{min} to {max}".
  templates.sort((a, b) => b.specificity - a.specificity);
}

function indexTemplate(text: string, source: string) {
  if (!/\{[A-Za-z_]\w*\}|%[sd]/.test(text) || indexedTemplates.has(text)) return;
  const specificity = text.replace(parameter, "").trim().length;
  // A bare parameter must not turn arbitrary authored text into system copy.
  if (specificity < 2) return;
  const keys: string[] = [];
  let end = 0, pattern = "^", ordinal = 0;
  for (const match of text.matchAll(parameter)) {
    pattern += escapeExpression(text.slice(end, match.index)) + "([\\s\\S]+?)";
    keys.push(match[1] || `printf${ordinal++}`);
    end = match.index! + match[0].length;
  }
  pattern += escapeExpression(text.slice(end)) + "$";
  indexedTemplates.add(text);
  templates.push({ expression: new RegExp(pattern), keys, source, specificity });
}

function interpolate(text: string, values: TranslationValues): string {
  let ordinal = 0;
  return text.replace(parameter, (placeholder, named: string | undefined) => {
    const key = named || `printf${ordinal++}`;
    return Object.prototype.hasOwnProperty.call(values, key) ? String(values[key]) : placeholder;
  });
}

function systemText(key: string, locale: Locale, source: string, settlementCurrency?: string): string {
  const text = dictionaries.get(locale)?.[key] || (locale === "zh" ? key : source);
  if (!settlementCurrency || !key.includes("结算币种") || currencyConcepts.has(key)) return text;
  // Replace the catalog template before interpolation, preserving names and
  // explicit original currencies supplied as values or authored text.
  return text.replace(/结算币种|settlement currency|決済通貨|валют(?:а|ы|е|у|ой) расч[её]тов|निपटान मुद्रा|mata wang penyelesaian|สกุลเงินที่ใช้ชำระบัญชี/gi, settlementCurrency);
}

export function translate(locale: Locale, source: string, values?: TranslationValues, settlementCurrency?: string): string {
  const entry = sources.has(source) ? source : reverseMessages.get(source);
  if (entry) { const text = systemText(entry, locale, source, settlementCurrency); return values ? interpolate(text, values) : text; }
  if (!values && source.length <= 20000) {
    const requirement = "仍有未满足事项：";
    const prefixes = [requirement, ...Array.from(dictionaries.values(), dictionary => dictionary[requirement]).filter(Boolean)];
    for (const prefix of prefixes) {
      if (source.startsWith(prefix)) {
        const items = source.slice(prefix.length).split(/；|\n/);
        return systemText(requirement, locale, requirement, settlementCurrency) + items.map(item => translate(locale, item, undefined, settlementCurrency)).join("\n");
      }
    }
    for (const template of templates) {
      const match = source.match(template.expression);
      if (match) return interpolate(systemText(template.source, locale, source, settlementCurrency), Object.fromEntries(template.keys.map((key, index) => [key, match[index + 1]])));
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
  return normalizeLocale(document.documentElement.lang) || normalizeLocale(cookie?.slice(localeCookie.length + 1)) || "zh";
}

// Localize display grouping/separators without converting financial strings to
// floating point or changing the original value copied/submitted to the API.
export function localizeNumberText(value: string, locale: Locale): string {
  if (locale === "hi") return value.replace(/\d{1,3}(?:,\d{3})+/g, grouped => {
    const digits = grouped.replaceAll(",", ""), leading = digits.slice(0, -3);
    return leading.replace(/\B(?=(\d{2})+(?!\d))/g, ",") + "," + digits.slice(-3);
  });
  if (locale !== "ru") return value;
  return value.replace(/[,\.]/g, token => token === "," ? "\u00a0" : ",");
}
