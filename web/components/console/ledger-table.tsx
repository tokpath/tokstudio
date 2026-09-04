import type { ReactNode } from "react";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { ScrollTable } from "@/components/ui/scroll-table";

/** 控制台账本表：ofox 明细密度，皮肤走 DESIGN.md 细线 + 悬停；宽表钉住首末列。 */
export function LedgerTable({
  columns,
  rows,
  emptyTitle,
  emptyDetail,
  stickyEnds = true,
}: {
  columns: string[];
  rows: { key: string; cells: ReactNode[] }[];
  emptyTitle: string;
  emptyDetail: string;
  stickyEnds?: boolean;
}) {
  if (rows.length === 0) {
    return <EmptyLedger title={emptyTitle} detail={emptyDetail} />;
  }
  return (
    <ScrollTable
      className="mt-3"
      density="ledger"
      stickyEnds={stickyEnds}
      minWidthClassName="min-w-[40rem]"
      columns={columns.map((col, i) => ({
        id: `${col}-${i}`,
        header: col,
        cell: (row: { key: string; cells: ReactNode[] }) => row.cells[i],
      }))}
      rows={rows}
      getRowId={(row) => row.key}
    />
  );
}
