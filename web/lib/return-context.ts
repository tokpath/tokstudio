/** A return link must stay within the same console surface. */
export function safeReturnHref(value: string | null | undefined, fallback: string): string {
  if (!value || !value.startsWith("/") || value.startsWith("//") || /[\\\r\n]/.test(value) || /%5c/i.test(value)) return fallback;
  const surface = fallback.split("/")[1];
  try { const url = new URL(value, "https://console.invalid"); return url.origin === "https://console.invalid" && (url.pathname === `/${surface}` || url.pathname.startsWith(`/${surface}/`)) ? `${url.pathname}${url.search}${url.hash}` : fallback; } catch { return fallback; }
}
export function appendReturnContext(href: string, returnTo: string): string {
  const url = new URL(href, "https://console.invalid");
  url.searchParams.set("return_to", returnTo);
  return `${url.pathname}${url.search}${url.hash}`;
}
