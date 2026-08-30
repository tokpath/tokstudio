import { cookies } from "next/headers";
import { getRequestConfig } from "next-intl/server";

export const locales = ["zh", "en", "ja"] as const;
export type Locale = (typeof locales)[number];

export default getRequestConfig(async () => {
  const cookieStore = await cookies();
  const raw = cookieStore.get("NEXT_LOCALE")?.value || "zh";
  const locale: Locale = locales.includes(raw as Locale) ? (raw as Locale) : "zh";
  return {
    locale,
    messages: (await import(`../messages/${locale}.json`)).default,
  };
});
