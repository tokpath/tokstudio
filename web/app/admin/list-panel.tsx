"use client";

import { useQuery } from "@tanstack/react-query";
import { flexRender, getCoreRowModel, useReactTable, type ColumnDef } from "@tanstack/react-table";
import { apiClient } from "@/lib/client";

type ListResponse<T> = { items?: T[]; error?: { message?: string } };

export function AdminListPanel<T extends Record<string, unknown>>({
  path,
  title,
  columns,
}: {
  path: string;
  title: string;
  columns: ColumnDef<T, unknown>[];
}) {
  const query = useQuery({
    queryKey: [path],
    queryFn: () => apiClient<ListResponse<T>>("GET", path),
  });
  const data = query.data?.items ?? [];
  const table = useReactTable({ data, columns, getCoreRowModel: getCoreRowModel() });
  return (
    <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
      <h2 className="mb-3 text-xl font-medium">{title}</h2>
      {query.isError || query.data?.error ? (
        <p className="text-sm text-slate-400">{query.data?.error?.message || "需要平台管理员登录后才能加载。"}</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="min-w-full text-left text-sm">
            <thead>
              {table.getHeaderGroups().map((group) => (
                <tr key={group.id} className="border-b border-slate-800 text-slate-400">
                  {group.headers.map((header) => (
                    <th key={header.id} className="px-2 py-2 font-medium">
                      {flexRender(header.column.columnDef.header, header.getContext())}
                    </th>
                  ))}
                </tr>
              ))}
            </thead>
            <tbody>
              {table.getRowModel().rows.map((row) => (
                <tr key={row.id} className="border-b border-slate-800/80">
                  {row.getVisibleCells().map((cell) => (
                    <td key={cell.id} className="px-2 py-2 text-slate-200">
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
