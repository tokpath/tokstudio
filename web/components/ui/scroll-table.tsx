"use client";

import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { stickyColumnClass, type ScrollTableDensity } from "@/lib/scroll-table";

export type ScrollTableColumn<T> = {
  id: string;
  header: ReactNode;
  cell: (row: T) => ReactNode;
  className?: string;
  /** false 时不钉；默认由 stickyEnds（首末列）决定。 */
  sticky?: false;
};

type ScrollTableProps<T> = {
  columns: ScrollTableColumn<T>[];
  rows: T[];
  getRowId: (row: T) => string;
  empty?: ReactNode;
  /** 默认 true：列数 ≥ 3 时钉住第一列与最后一列。 */
  stickyEnds?: boolean;
  density?: ScrollTableDensity;
  minWidthClassName?: string;
  onRowClick?: (row: T) => void;
  rowClassName?: (row: T) => string | undefined;
  className?: string;
};

/** 横滑宽表：默认钉住第一列与最后一列，中间列可滑动。对齐 DESIGN.md §4.5。 */
export function ScrollTable<T>({
  columns,
  rows,
  getRowId,
  empty,
  stickyEnds = true,
  density = "admin",
  minWidthClassName = "min-w-[52rem]",
  onRowClick,
  rowClassName,
  className,
}: ScrollTableProps<T>) {
  const count = columns.length;

  function cellClass(index: number, header: boolean, extra?: string) {
    const col = columns[index];
    const pinEnds = stickyEnds && col.sticky !== false;
    return cn(stickyColumnClass(index, count, { stickyEnds: pinEnds, density, header }), extra, col.className);
  }

  return (
    <div className={cn("overflow-x-auto", className)}>
      <table className={cn("w-full text-left text-sm", minWidthClassName)}>
        <thead>
          <tr className="group border-b border-hairline">
            {columns.map((col, index) => (
              <th key={col.id} className={cellClass(index, true)}>
                {col.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 ? (
            <tr>
              <td colSpan={Math.max(count, 1)} className="px-3 py-6">
                {empty}
              </td>
            </tr>
          ) : (
            rows.map((row) => (
              <tr
                key={getRowId(row)}
                className={cn(
                  "group border-b border-hairline hover:bg-brand-soft/40",
                  onRowClick && "cursor-pointer",
                  rowClassName?.(row),
                )}
                onClick={onRowClick ? () => onRowClick(row) : undefined}
              >
                {columns.map((col, index) => (
                  <td
                    key={`${getRowId(row)}-${col.id}`}
                    className={cellClass(index, false, density === "ledger" ? "font-mono text-[13px]" : undefined)}
                  >
                    {col.cell(row)}
                  </td>
                ))}
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}
