import type { StatusBadge as StatusBadgeData } from "@/lib/status";

const toneClass: Record<StatusBadgeData["tone"], string> = {
  success: "text-success",
  hold: "text-hold",
  degraded: "text-degraded",
  danger: "text-danger",
};

/** 状态必须带字，不能只靠色点。 */
export function StatusBadge({ badge }: { badge: StatusBadgeData }) {
  return <span className={`th-eyebrow ${toneClass[badge.tone]}`}>{badge.word}</span>;
}
