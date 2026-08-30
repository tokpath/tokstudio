import en from "@/messages/en.json";
import ja from "@/messages/ja.json";
import zh from "@/messages/zh.json";

export const locales = ["zh", "en", "ja"] as const;
export type Locale = (typeof locales)[number];

const catalog = { zh, en, ja };

export function resolveLocale(value?: string | null): Locale {
  if (value === "en" || value === "ja" || value === "zh") {
    return value;
  }
  return "zh";
}

export function messagesFor(locale?: string | null) {
  return catalog[resolveLocale(locale)];
}

export function translate(locale: string | null | undefined, key: string): string {
  const messages = messagesFor(locale) as Record<string, Record<string, string>>;
  const [group, name] = key.split(".");
  return messages[group]?.[name] || key;
}
