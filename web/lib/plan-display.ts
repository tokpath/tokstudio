import { defaultCopy, type CopyTranslator } from "@/lib/copy";
import { USD_CREDIT, formatUsdMinor } from "@/lib/money";

export function planPeriodLabels(tUI: CopyTranslator = defaultCopy): Record<string, string> { return {
  lifetime: tUI("user.planLifetime"),
  monthly: tUI("user.planMonthly"),
  quarterly: tUI("keyUX.periodQuarter"),
  yearly: tUI("publicExperience.period_year"),
}; }
export const planPeriodLabel = planPeriodLabels();

export function planStatusLabels(tUI: CopyTranslator = defaultCopy): Record<string, string> { return {
  pending_review: tUI("common.stPendingReview"),
  published: tUI("common.stPublished"),
  archived: tUI("common.stArchived"),
  rejected: tUI("common.stRejected"),
}; }
export const planStatusLabel = planStatusLabels();

export function planQuotaText(items?: { unit_type?: string; included_amount?: number }[], tUI: CopyTranslator = defaultCopy) {
  const text = items?.map((item) => {
    const amount = item.unit_type === USD_CREDIT ? formatUsdMinor(item.included_amount) : (item.included_amount ?? 0).toLocaleString();
    const unit = item.unit_type && item.unit_type !== USD_CREDIT ? ` ${item.unit_type}` : "";
    return `${amount}${unit}`;
  }).filter(Boolean).join("、");
  return text || "—";
}

export function planAudienceText(
  plan: { channel_scope?: string; channel_ids?: string[]; owner_type?: string },
  names: Map<string, string>, tUI: CopyTranslator = defaultCopy
) {
  if (plan.channel_scope === "selected") {
    const labels = (plan.channel_ids ?? []).map((id) => names.get(id) || id).filter(Boolean);
    return labels.length ? labels.join("、") : "—";
  }
  // 文案都是「所有渠道」。平台套餐指平台下的渠道，OEM 套餐指该 OEM 下的客户。
  return tUI("consoleCopy.allChannels226");
}
