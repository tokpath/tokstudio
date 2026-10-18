// Carry only supported customer tasks across authentication. Query parameters
// are rebuilt from a route-specific allowlist; invitation attribution stays in
// the registration flow and is never inferred from a continuation URL.
export function safeCustomerNext(value?: string | null): string | undefined {
  if (!value || value.length > 2048 || /[\u0000-\u0020\u007f\\#]/.test(value) || /%(?![\da-f]{2})/i.test(value)) return;
  const match = value.match(/^(\/workspace\/(models|plans|playground))(?:\?[^#]*)?$/);
  if (!match) return;
  const source = new URLSearchParams(value.split("?").slice(1).join("?"));
  const result = new URLSearchParams();
  const keys = match[2] === "plans" ? ["plan", "kol_activity"] : ["model"];
  for (const key of keys) {
    const entry = source.get(key);
    if (entry && !/[\u0000-\u001f\u007f\\]/.test(entry)) result.set(key, entry);
  }
  return `${match[1]}${result.size ? `?${result}` : ""}`;
}
export function authContinuation(path: string, next?: string, invite?: string) {
  const query = new URLSearchParams();
  if (next) query.set("next", next);
  if (invite) query.set("invite", invite);
  return `${path}${query.size ? `?${query}` : ""}`;
}
