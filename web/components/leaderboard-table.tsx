import Link from "next/link";
import { Trophy } from "lucide-react";
import type { LeaderboardRow } from "@/lib/site-content";

export function LeaderboardTable({
  rows,
  hrefOf,
}: {
  rows: LeaderboardRow[];
  hrefOf?: (row: LeaderboardRow) => string;
}) {
  return (
    <ol className="divide-y divide-hairline rounded-card border border-hairline bg-canvas-raised">
      {rows.map((row) => {
        const href = hrefOf ? hrefOf(row) : row.id ? `/models/${row.id}` : "/models";
        return (
          <li key={row.rank + row.name}>
            <Link href={href} className="flex items-center gap-4 px-4 py-3 no-underline hover:bg-brand-soft/30">
              <span className="inline-flex w-8 items-center justify-center font-mono text-sm text-ink-mute">
                {row.rank === "1" ? <Trophy className="size-4 text-hold" strokeWidth={1.75} aria-hidden /> : row.rank}
              </span>
              <div className="min-w-0 flex-1">
                {row.vendor ? <p className="text-[12px] text-ink-mute">{row.vendor}</p> : null}
                <p className="font-medium text-ink">{row.name}</p>
              </div>
              <span className="font-mono tabular-nums text-ink">{row.share}</span>
              <span className="w-24 text-right font-mono text-[12px] tabular-nums text-ink-mute">{row.delta}</span>
            </Link>
          </li>
        );
      })}
    </ol>
  );
}
