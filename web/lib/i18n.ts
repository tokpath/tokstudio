import en from "@/messages/en.json";
import ja from "@/messages/ja.json";
import zh from "@/messages/zh.json";

export const locales = ["zh", "en", "ja"] as const;
export type Locale = (typeof locales)[number];

const catalog = { zh, en, ja };

export function localeFromCookie(value?: string | null): Locale | null {
  if (value === "en" || value === "ja" || value === "zh") {
    return value;
  }
  return null;
}

/** 从 Accept-Language 选 zh / en / ja。按 q 值排序，前缀匹配（zh-CN → zh）。 */
export function parseAcceptLanguage(header?: string | null): Locale | null {
  if (!header) {
    return null;
  }
  const parts = header.split(",").map((part) => {
    const [tagRaw, ...params] = part.trim().split(";");
    const qParam = params.find((item) => item.trim().startsWith("q="));
    const quality = qParam ? Number(qParam.trim().slice(2)) : 1;
    return { tag: (tagRaw || "").trim().toLowerCase(), quality: Number.isFinite(quality) ? quality : 0 };
  });
  parts.sort((a, b) => b.quality - a.quality);
  for (const { tag } of parts) {
    if (tag === "*" || !tag) {
      continue;
    }
    if (tag === "zh" || tag.startsWith("zh-")) {
      return "zh";
    }
    if (tag === "ja" || tag.startsWith("ja-")) {
      return "ja";
    }
    if (tag === "en" || tag.startsWith("en-")) {
      return "en";
    }
  }
  return null;
}

/** cookie 是用户显式选择；没有 cookie 时用 Accept-Language；都没有则中文。不用 URL 区分语言。 */
export function resolveRequestLocale(cookie?: string | null, acceptLanguage?: string | null): Locale {
  return localeFromCookie(cookie) || parseAcceptLanguage(acceptLanguage) || "zh";
}

export function resolveLocale(value?: string | null): Locale {
  return localeFromCookie(value) || "zh";
}

export function messagesFor(locale?: string | null) {
  return catalog[resolveLocale(locale)];
}

export function messagesForRequest(cookie?: string | null, acceptLanguage?: string | null) {
  return catalog[resolveRequestLocale(cookie, acceptLanguage)];
}

export function htmlLang(locale: Locale) {
  if (locale === "zh") {
    return "zh-CN";
  }
  if (locale === "ja") {
    return "ja";
  }
  return "en";
}

export function translate(locale: string | null | undefined, key: string): string {
  const messages = messagesFor(locale) as unknown as Record<string, Record<string, string>>;
  const [group, name] = key.split(".");
  return messages[group]?.[name] || key;
}
