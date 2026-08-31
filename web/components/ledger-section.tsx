import { CheckRow } from "@/components/check-row";
import { EmptyState } from "@/components/empty-state";
import { Eyebrow } from "@/components/eyebrow";

export function LedgerSection({
  title,
  eyebrow,
  rows,
  emptyTitle,
  emptyDetail,
}: {
  title: string;
  eyebrow: string;
  rows: { key: string; label: string; value: string }[];
  emptyTitle: string;
  emptyDetail: string;
}) {
  return (
    <section className="overflow-hidden rounded-card border border-hairline bg-canvas-raised">
      <div className="flex items-center justify-between border-b border-hairline px-4 py-3">
        <h2 className="text-lg font-semibold">{title}</h2>
        <Eyebrow>{eyebrow}</Eyebrow>
      </div>
      {rows.length === 0 ? (
        <EmptyState title={emptyTitle} detail={emptyDetail} />
      ) : (
        rows.map((row) => <CheckRow key={row.key} label={row.label} value={row.value} />)
      )}
    </section>
  );
}
