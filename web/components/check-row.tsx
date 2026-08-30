import { StatusBadge } from "@/components/status-badge";
import { badgeForCheck, classifyCheck } from "@/lib/status";

export function CheckRow({ label, value }: { label: string; value: string }) {
  const badge = badgeForCheck(classifyCheck(value));

  return (
    <div className="flex items-center justify-between gap-4 border-b border-hairline px-4 py-3 last:border-b-0">
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="text-sm text-ink">{label}</span>
        <span className="font-mono text-[13px] text-ink-mute">{value}</span>
      </div>
      <StatusBadge badge={badge} />
    </div>
  );
}
