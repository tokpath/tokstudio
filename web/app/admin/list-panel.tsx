"use client";

import { useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { flexRender, getCoreRowModel, useReactTable, type ColumnDef } from "@tanstack/react-table";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { EmptyState } from "@/components/empty-state";
import { useTranslations } from "next-intl";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";

type ListResponse<T> = { items?: T[]; error?: { message?: string } };

export function AdminListPanel<T extends Record<string, unknown>>({
  path,
  title,
  columns,
  rowHref,
  onRowSelect,
  rowSelected,
  actions,
  emptyTitle = "暂无记录",
  emptyDetail = "登录平台管理员后可以看到数据。",
}: {
  path: string;
  title: string;
  columns: ColumnDef<T, unknown>[];
  rowHref?: (row: T) => string;
  onRowSelect?: (row: T) => void;
  rowSelected?: (row: T) => boolean;
  actions?: ReactNode;
  emptyTitle?: string;
  emptyDetail?: string;
}) {
  const router = useRouter();
  const tc = useTranslations("common");
  const [q, setQ] = useState("");
  const href = q ? `${path}${path.includes("?") ? "&" : "?"}q=${encodeURIComponent(q)}` : path;
  const query = useQuery({
    queryKey: [href],
    queryFn: () => apiClient<ListResponse<T>>("GET", href),
  });
  const data = query.data?.items ?? [];
  const table = useReactTable({ data, columns, getCoreRowModel: getCoreRowModel() });
  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-lg font-semibold tracking-tight">{title}</h2>
        <div className="flex flex-wrap items-center gap-2">
          {actions}
          <Input placeholder={tc("filter")} value={q} onChange={(e) => setQ(e.target.value)} />
          <Button
            size="sm"
            variant="outline"
            onClick={async () => {
              const sep = href.includes("?") ? "&" : "?";
              const response = await fetch(`${apiBase}${href}${sep}format=csv&limit=100`, { credentials: "include" });
              const blob = await response.blob();
              const url = URL.createObjectURL(blob);
              const a = document.createElement("a");
              a.href = url;
              a.download = "export.csv";
              a.click();
              URL.revokeObjectURL(url);
            }}
          >
            {tc("exportCsv")}
          </Button>
        </div>
      </div>
      {query.isError || query.data?.error ? (
        <p className="mb-3 text-sm text-ink-secondary">{query.data?.error?.message || tc("needAdmin")}</p>
      ) : null}
      <div className="overflow-x-auto">
        <table className="min-w-full text-left text-sm">
          <thead>
            {table.getHeaderGroups().map((group) => (
              <tr key={group.id} className="border-b border-hairline">
                {group.headers.map((header) => (
                  <th key={header.id} className="th-eyebrow px-3 py-2.5 text-ink-mute">
                    {flexRender(header.column.columnDef.header, header.getContext())}
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody>
            {data.length === 0 ? (
              <tr>
                <td colSpan={Math.max(columns.length, 1)} className="px-3 py-6">
                  <EmptyState
                    title={query.isError || query.data?.error ? "暂时看不到数据" : emptyTitle}
                    detail={query.isError || query.data?.error ? tc("needAdmin") : emptyDetail}
                  />
                </td>
              </tr>
            ) : (
              table.getRowModel().rows.map((row) => (
                <tr
                  key={row.id}
                  className={`border-b border-hairline hover:bg-brand-soft/40 ${
                    rowHref || onRowSelect ? "cursor-pointer" : ""
                  } ${rowSelected?.(row.original) ? "bg-brand-soft" : ""}`}
                  onClick={() => {
                    if (onRowSelect) {
                      onRowSelect(row.original);
                      return;
                    }
                    if (rowHref) {
                      router.push(rowHref(row.original));
                    }
                  }}
                >
                  {row.getVisibleCells().map((cell) => (
                    <td key={cell.id} className="px-3 py-2.5 text-ink">
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </td>
                  ))}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </section>
  );
}
