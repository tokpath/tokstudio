import { cookies } from "next/headers";
import { getRequestConfig } from "next-intl/server";
import { resolveLocale } from "../lib/i18n";

export default getRequestConfig(async () => {
  const cookieStore = await cookies();
  const locale = resolveLocale(cookieStore.get("NEXT_LOCALE")?.value);
  return {
    locale,
    messages: (await import(`../messages/${locale}.json`)).default,
  };
});
