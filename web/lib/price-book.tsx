import { formatIOPerMillion } from "@/lib/token-price";
import type { ColumnDef } from "@tanstack/react-table";

export type PriceBook = {
  id: string;
  public_id: string;
  status: string;
  effective_at?: string;
  sell?: string;
  unit_prices?: Record<string, unknown>;
  [key: string]: unknown;
};

const tabular = "font-mono text-xs tabular-nums";

function PriceCell({ value }: { value?: string }) {
  return <span className={tabular}>{value || "—"}</span>;
}

export const priceBookColumns: ColumnDef<PriceBook, unknown>[] = [
  { accessorKey: "id", header: "Version", cell: ({ row }) => <span className="font-mono text-xs">{row.original.id}</span> },
  { accessorKey: "public_id", header: "Model", cell: ({ row }) => <span className="font-mono text-xs">{row.original.public_id}</span> },
  { accessorKey: "status", header: "Status" },
  {
    accessorKey: "effective_at",
    header: "Effective",
    cell: ({ row }) => <span className={tabular}>{row.original.effective_at || "—"}</span>,
  },
  { accessorKey: "sell", header: "Sell /M", cell: ({ row }) => <PriceCell value={formatIOPerMillion(row.original.sell)} /> },
];

export function publishedPriceLabel(
  price: { public_id?: string; PublicID?: string; version_id?: string; VersionID?: string } | undefined,
  fallback: string,
) {
  return price?.public_id || price?.PublicID || price?.version_id || price?.VersionID || fallback;
}
