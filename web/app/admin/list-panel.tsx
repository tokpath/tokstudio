"use client";

import { useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { flexRender, getCoreRowModel, useReactTable, type ColumnDef } from "@tanstack/react-table";
import { Input } from "@/components/ui/input";
import { EmptyState } from "@/components/empty-state";
import { useTranslations } from "next-intl";
import { apiClient } from "@/lib/client";
import { stickyColumnClass } from "@/lib/scroll-table";
import { cn } from "@/lib/utils";

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
  stickyEnds = true,
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
  stickyEnds?: boolean;
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
  const colCount = columns.length;

  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-lg font-semibold tracking-tight">{title}</h2>
        <div className="flex shrink-0 items-center gap-2">
          {actions}
          <Input
            className="w-44 sm:w-56"
            placeholder={tc("filter")}
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
      </div>
      {query.isError || query.data?.error ? (
        <p className="mb-3 text-sm text-ink-secondary">{query.data?.error?.message || tc("needAdmin")}</p>
      ) : null}
      <div className="overflow-x-auto">
        <table className="min-w-[52rem] w-full text-left text-sm">
          <thead>
            {table.getHeaderGroups().map((group) => (
              <tr key={group.id} className="group border-b border-hairline">
                {group.headers.map((header, index) => (
                  <th key={header.id} className={stickyColumnClass(index, colCount, { stickyEnds, header: true })}>
                    {flexRender(header.column.columnDef.header, header.getContext())}
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody>
            {data.length === 0 ? (
              <tr>
                <td colSpan={Math.max(colCount, 1)} className="px-3 py-6">
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
                  className={cn(
                    "group border-b border-hairline hover:bg-brand-soft/40",
                    rowHref || onRowSelect ? "cursor-pointer" : "",
                    rowSelected?.(row.original) ? "bg-brand-soft" : "",
                  )}
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
                  {row.getVisibleCells().map((cell, index) => (
                    <td key={cell.id} className={stickyColumnClass(index, colCount, { stickyEnds })}>
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
