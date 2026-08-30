"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { flexRender, getCoreRowModel, useReactTable, type ColumnDef } from "@tanstack/react-table";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
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
  const [q, setQ] = useState("");
  const href = q ? `${path}${path.includes("?") ? "&" : "?"}q=${encodeURIComponent(q)}` : path;
  const query = useQuery({
    queryKey: [href],
    queryFn: () => apiClient<ListResponse<T>>("GET", href),
  });
  const data = query.data?.items ?? [];
  const table = useReactTable({ data, columns, getCoreRowModel: getCoreRowModel() });
  return (
    <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-xl font-medium">{title}</h2>
        <div className="flex flex-wrap items-center gap-2">
          <Input placeholder="筛选" value={q} onChange={(e) => setQ(e.target.value)} />
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
          导出 CSV
        </Button>
        </div>
      </div>
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
