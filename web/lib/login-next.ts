/** 只允许站内相对路径，避免开放重定向到外站。可带查询串。 */
export function safeNextPath(raw?: string | null): string {
  if (!raw) {
    return "";
  }
  const value = raw.trim();
  if (!value.startsWith("/") || value.startsWith("//") || /[\\\u0000-\u001f\u007f]/.test(value)) return "";
  try {
    const url = new URL(value, "https://return.invalid");
    if (url.origin !== "https://return.invalid") return "";
    let path = value.split(/[?#]/)[0];
    for (let i = 0; i < 3; i++) {
      if (path.startsWith("//") || /[\\\u0000-\u001f\u007f]/.test(path)) return "";
      const decoded = decodeURIComponent(path);
      if (decoded === path) break;
      path = decoded;
    }
  } catch { return ""; }
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
