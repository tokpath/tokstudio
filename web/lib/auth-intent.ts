import { safeNextPath } from "./login-next";

export type AuthIntent = { next: string; promotionCode: string };
export const AUTH_INTENT_KEY = "tokenhub_auth_intent_v1";
const MAX_AGE = 15 * 60 * 1000;
type IntentStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;

export function promotionCode(raw?: string | null): string {
  const code = (raw || "").trim().toUpperCase();
  return /^[A-Z0-9_-]{1,64}$/.test(code) ? code : "";
}
export function authIntent(search: Pick<URLSearchParams, "get">): AuthIntent {
  return { next: safeNextPath(search.get("next")), promotionCode: promotionCode(search.get("promotion_code") || search.get("promo")) };
}
export function storeAuthIntent(storage: IntentStorage | null | undefined, intent: AuthIntent, now = Date.now()) {
  if (!storage) return;
  try { storage.setItem(AUTH_INTENT_KEY, JSON.stringify({ next: safeNextPath(intent.next), promotionCode: promotionCode(intent.promotionCode), at: now })); } catch { /* private mode */ }
}
export function readAuthIntent(storage: IntentStorage | null | undefined, consume = false, now = Date.now()): AuthIntent {
  const empty = { next: "", promotionCode: "" };
  if (!storage) return empty;
  try {
    const raw = storage.getItem(AUTH_INTENT_KEY);
    if (consume) storage.removeItem(AUTH_INTENT_KEY);
    if (!raw) return empty;
    const value = JSON.parse(raw);
    if (typeof value.at !== "number" || value.at > now || now - value.at > MAX_AGE) { storage.removeItem(AUTH_INTENT_KEY); return empty; }
    return { next: safeNextPath(typeof value.next === "string" ? value.next : ""), promotionCode: promotionCode(typeof value.promotionCode === "string" ? value.promotionCode : "") };
  } catch { return empty; }
}

export function loginIntentHref(next: string, rawPromo?: string | null): string {
  const params = new URLSearchParams({ next: safeNextPath(next) || "/app" });
  const code = promotionCode(rawPromo);
  if (code) params.set("promotion_code", code);
  return `/login?${params}`;
}
