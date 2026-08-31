function trimSlash(url: string): string {
  return url.replace(/\/$/, "");
}

/** 浏览器同源前缀。Caddy / Next rewrite 会把 /api 剥掉再交给 Go。 */
export function resolveBrowserApiBase(value?: string): string {
  const raw = (value ?? "/api").trim();
  if (raw === "") {
    return "/api";
  }
  return trimSlash(raw);
}

/** RSC / 服务端直连 Go。Compose 内网是 http://api:8080。 */
export function resolveServerApiBase(internal?: string, publicBase?: string): string {
  const raw = (internal || publicBase || "http://127.0.0.1:8080").trim();
  if (raw === "") {
    return "http://127.0.0.1:8080";
  }
  return trimSlash(raw);
}

export const apiBase = resolveBrowserApiBase(process.env.NEXT_PUBLIC_API_BASE_URL);
export const serverApiBase = resolveServerApiBase(
  process.env.TOKENHUB_API_INTERNAL_URL,
  process.env.TOKENHUB_PUBLIC_BASE_URL,
);

export async function fetchAPI<T>(path: string, init?: RequestInit & { host?: string }): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.host) {
    headers.set("X-Forwarded-Host", init.host);
  }
  const response = await fetch(`${serverApiBase}${path}`, {
    ...init,
    headers,
    cache: "no-store",
  });
  return (await response.json()) as T;
}
