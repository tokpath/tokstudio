import type { ReactNode } from "react";

/** 表头用 eyebrow，空表不画假行。 */
export function CatalogFrame({
  columns,
  children,
}: {
  columns: string[];
  children: ReactNode;
}) {
  return (
    <section className="overflow-hidden rounded-stamp border border-hairline bg-canvas-raised">
      <div className="overflow-x-auto">
        <div
          className="hidden min-w-[640px] border-b border-hairline px-4 py-3 md:grid"
          style={{ gridTemplateColumns: `repeat(${columns.length}, minmax(0, 1fr))` }}
        >
          {columns.map((column) => (
            <p key={column} className="th-eyebrow text-ink-mute">
              {column}
            </p>
          ))}
        </div>
      </div>
      {children}
    </section>
  );
}
