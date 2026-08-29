/** 只允许站内相对路径，避免开放重定向到外站。 */
export function safeNextPath(raw?: string | null): string {
  if (!raw) {
    return "";
  }
  const value = raw.trim();
  if (!value.startsWith("/") || value.startsWith("//") || value.includes("://")) {
    return "";
  }
  return value;
}

export function loginHref(next = "/"): string {
  const path = safeNextPath(next) || "/";
  return `/login?next=${encodeURIComponent(path)}`;
}
