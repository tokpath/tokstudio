export const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080";

export async function fetchAPI<T>(path: string, init?: RequestInit & { host?: string }): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.host) {
    headers.set("X-Forwarded-Host", init.host);
  }
  const response = await fetch(`${apiBase}${path}`, {
    ...init,
    headers,
    cache: "no-store",
  });
  return (await response.json()) as T;
}
