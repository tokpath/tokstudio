import { StatusBadge } from "@/components/status-badge";
import { badgeForCheck, classifyCheck, isOperationalValue } from "@/lib/status";

export function CheckRow({ label, value }: { label: string; value: string }) {
  const showBadge = isOperationalValue(value);

  return (
    <div className="flex items-center justify-between gap-4 border-b border-hairline px-4 py-3 last:border-b-0 hover:bg-brand-soft/40">
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="text-sm text-ink">{label}</span>
        <span className="truncate font-mono text-[13px] tabular-nums text-ink-mute">{value}</span>
      </div>
      {showBadge ? <StatusBadge badge={badgeForCheck(classifyCheck(value))} /> : null}
    </div>
  );
}
