import { USD_CREDIT, formatUsdMinor } from "@/lib/money";

export const planPeriodLabel: Record<string, string> = {
  lifetime: "不限时",
  monthly: "每月",
  quarterly: "每季",
  yearly: "每年",
};

export const planStatusLabel: Record<string, string> = {
  pending_review: "待审核",
  published: "已发布",
  archived: "已下架",
  rejected: "已拒绝",
};

export function planQuotaText(items?: { unit_type?: string; included_amount?: number }[]) {
  const text = items?.map((item) => {
    const amount = item.unit_type === USD_CREDIT ? formatUsdMinor(item.included_amount) : (item.included_amount ?? 0).toLocaleString();
    const unit = item.unit_type && item.unit_type !== USD_CREDIT ? ` ${item.unit_type}` : "";
    return `${amount}${unit}`;
  }).filter(Boolean).join("、");
  return text || "—";
}

export function planAudienceText(
  plan: { channel_scope?: string; channel_ids?: string[]; owner_type?: string },
  names: Map<string, string>,
) {
  if (plan.channel_scope === "selected") {
    const labels = (plan.channel_ids ?? []).map((id) => names.get(id) || id).filter(Boolean);
    return labels.length ? labels.join("、") : "—";
  }
  // 文案都是「所有渠道」。平台套餐指平台下的渠道，OEM 套餐指该 OEM 下的客户。
  return "所有渠道";
}
