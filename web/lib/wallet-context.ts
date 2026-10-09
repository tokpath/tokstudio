import type { CheckoutOrder } from "./checkout";
import { safeNextPath } from "./login-next";
export function walletReturnPath(raw: string | null | undefined) {
  const next = safeNextPath(raw);
  if (!next) return "";
  let path = new URL(next, "https://wallet.invalid").pathname.replace(/\/$/, "");
  try { for(let i=0;i<3;i++){const decoded=decodeURIComponent(path);if(decoded===path)break;path=decoded;} } catch {return "";}
  return ["/app/wallet", "/app/plans", "/console/wallet", "/console/plans"].includes(path) ? "" : next;
}
export function walletTabHref(search: string, tab: string) {
  const params = new URLSearchParams(search);
  params.set("tab", tab);
  params.delete("cursor"); params.delete("record_kind"); params.delete("record_scope");
  const next = walletReturnPath(params.get("next"));
  if (next) params.set("next", next); else params.delete("next");
  return `/app/wallet?${params}`;
}

export function purchaseIsResolved(order: CheckoutOrder) {
  return ["failed","expired","refunded","partially_refunded"].includes(order.status || "") || (order.status === "paid" && !!order.fulfilled_at);
}
