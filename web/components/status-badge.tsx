import type { StatusBadge as StatusBadgeData } from "@/lib/status";

const toneClass: Record<StatusBadgeData["tone"], string> = {
  success: "text-success",
  hold: "text-hold",
  degraded: "text-degraded",
  danger: "text-danger",
};

/** 状态必须带字，不能只靠色点。 */
export function StatusBadge({ badge }: { badge: StatusBadgeData }) {
  return (
    <span
      className={`font-mono text-[11px] font-medium uppercase tracking-[0.08em] ${toneClass[badge.tone]}`}
    >
      {badge.word}
    </span>
  );
}
