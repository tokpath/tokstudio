import type { ReactNode } from "react";
import { EmptyLedger } from "@/components/console/empty-ledger";

/** 控制台账本表：ofox 明细密度，皮肤走 DESIGN.md 细线 + 悬停。 */
export function LedgerTable({
  columns,
  rows,
  emptyTitle,
  emptyDetail,
}: {
  columns: string[];
  rows: { key: string; cells: ReactNode[] }[];
  emptyTitle: string;
  emptyDetail: string;
}) {
  if (rows.length === 0) {
    return <EmptyLedger title={emptyTitle} detail={emptyDetail} />;
  }
  return (
    <div className="mt-3 overflow-x-auto">
      <table className="min-w-full text-left text-sm">
        <thead>
          <tr className="border-b border-hairline">
            {columns.map((col) => (
              <th key={col} className="th-eyebrow px-4 py-3 text-ink-mute">
                {col}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.key} className="border-b border-hairline hover:bg-brand-soft/40">
              {row.cells.map((cell, i) => (
                <td key={`${row.key}-${i}`} className="px-4 py-3 font-mono text-[13px] text-ink">
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
