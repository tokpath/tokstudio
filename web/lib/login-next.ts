/** 只允许站内相对路径，避免开放重定向到外站。可带查询串。 */
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

export function pagePathWithSearch(pathname: string, search?: string | null): string {
  const path = safeNextPath(pathname.split("?")[0] || pathname) || "/";
  const query = (search || "").replace(/^\?/, "");
  if (!query) {
    return path;
  }
  return safeNextPath(`${path}?${query}`) || path;
}

export function loginHref(next = "/"): string {
  const path = safeNextPath(next) || "/";
  return `/login?next=${encodeURIComponent(path)}`;
}
