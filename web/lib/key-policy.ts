export type KeyPolicy = {
  model_mode?: "all" | "selected";
  allowlist?: string[];
  budget_limit_minor?: number | null;
  budget_used_minor?: number;
  budget_reserved_minor?: number;
  budget_period?: "lifetime" | "month" | "quarter" | "year" | "";
  status?: string;
  expires_at?: string | null;
};
export function usdToMinor(raw: string): number | null {
  if (!/^\d+(\.\d{1,6})?$/.test(raw.trim())) return null;
  const [whole, fraction = ""] = raw.trim().split(".");
  const result = Number(whole) * 1_000_000 + Number(fraction.padEnd(6, "0"));
  return Number.isSafeInteger(result) && result > 0 ? result : null;
}
export function keyAllowsModel(key: KeyPolicy, model: string): boolean {
  return (key.model_mode ?? (key.allowlist?.length ? "selected" : "all")) === "all" || Boolean(key.allowlist?.includes(model));
}
export function keyState(key: KeyPolicy, now = Date.now()): "active" | "disabled" | "expired" | "spent" | "occupied" {
  if (key.status === "disabled") return "disabled";
  if (key.expires_at && Date.parse(key.expires_at) <= now) return "expired";
  if (key.budget_limit_minor != null) {
    const used = key.budget_used_minor ?? 0;
    if (used >= key.budget_limit_minor) return "spent";
    if (used + (key.budget_reserved_minor ?? 0) >= key.budget_limit_minor) return "occupied";
  }
  return "active";
}

/** 余量用完就是 0，不显示负数。 */
export function keyRemainingMinor(key: KeyPolicy): number | null {
  if (key.budget_limit_minor == null) {
    return null;
  }
  const left = key.budget_limit_minor - (key.budget_used_minor ?? 0) - (key.budget_reserved_minor ?? 0);
  return left > 0 ? left : 0;
}
export function keyDocsHref(keyID: string, model = "", tab: "agent" | "protocol" = "protocol"): string {
  const params = new URLSearchParams({ key_id: keyID, tab });
  if (model) params.set("model", model);
  return `/app/docs?${params}`;
}

/** Keep a legal model context, then the only allowed model, otherwise ask the user. */
export function preferredKeyModel(key: KeyPolicy, context = ""): string {
  if (context && keyAllowsModel(key, context)) return context;
  return (key.model_mode ?? (key.allowlist?.length ? "selected" : "all")) === "selected" && key.allowlist?.length === 1 ? key.allowlist[0] : "";
}
