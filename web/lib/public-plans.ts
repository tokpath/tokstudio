import { serverApiBase } from "./api";
import { formatPayMinor } from "./payment-quote";

export type PublicPlan = {
  id: string; name: string; price_minor: number; currency: string; billing_period: string; auto_renew_allowed: boolean;
  items?: { unit_type: string; included_amount: number; public_model_id?: string; expires_in_seconds?: number }[];
};
export function planPeriod(period?: string): "lifetime" | "month" | "quarter" | "year" | "unknown" {
  if (period === "lifetime") return "lifetime";
  if (period === "monthly") return "month";
  if (period === "quarterly") return "quarter";
  if (period === "yearly") return "year";
  return "unknown";
}
export function planPrice(plan: Pick<PublicPlan,"currency"|"price_minor">) {
  return `${formatPayMinor(plan.currency,plan.price_minor)} ${plan.currency || "—"}`;
}
export async function loadPublicPlans(host: string): Promise<{items: PublicPlan[]; ok: boolean}> {
  try {
    const response = await fetch(`${serverApiBase}/v1/plans`, {headers: {"X-Forwarded-Host": host}, cache:"no-store"});
    const body = await response.json();
    return { items: response.ok && Array.isArray(body.items) ? body.items : [], ok: response.ok };
  } catch { return {items:[],ok:false}; }
}
