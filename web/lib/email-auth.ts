import { apiBase } from "@/lib/api";

export function emailTokenFromFragment(fragment: string): string {
  const token = new URLSearchParams(fragment.replace(/^#/, "")).get("token") || "";
  return /^themail_[a-f0-9]{48}$/.test(token) ? token : "";
}

export class EmailActionError extends Error {
  constructor(public code: string, public retryAfter = 0) { super(code); }
}

export async function emailAction(path: string, body: Record<string, string> = {}): Promise<{ retry_after?: number }> {
  const response = await fetch(`${apiBase}${path}`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new EmailActionError(data.error?.code || "failed", Number(response.headers?.get("Retry-After")) || 0);
  return data;
}

export function emailErrorKey(error: unknown): "unavailable" | "invalidLink" | "rateLimited" | "failed" | "networkError" {
  if (!(error instanceof EmailActionError)) return "networkError";
  switch (error.code) {
    case "email_unavailable": case "service_unavailable": return "unavailable";
    case "email_link_invalid": return "invalidLink";
    case "rate_limit_exceeded": return "rateLimited";
    default: return "failed";
  }
}
