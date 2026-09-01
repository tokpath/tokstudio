import { cookies, headers } from "next/headers";
import { getRequestConfig } from "next-intl/server";
import { resolveRequestLocale } from "@/lib/i18n";

export default getRequestConfig(async () => {
  const cookie = (await cookies()).get("NEXT_LOCALE")?.value;
  const accept = (await headers()).get("accept-language");
  const locale = resolveRequestLocale(cookie, accept);
  return {
    locale,
    messages: (await import(`../messages/${locale}.json`)).default,
  };
});
