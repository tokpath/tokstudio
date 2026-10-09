"use client";

import { useEffect, useState, type ReactNode } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { flexRender, getCoreRowModel, useReactTable, type ColumnDef } from "@tanstack/react-table";
import { Input } from "@/components/ui/input";
import { EmptyState } from "@/components/empty-state";
import { useTranslations } from "next-intl";
import { apiClient } from "@/lib/client";
import { stickyColumnClass, type ScrollTableDensity } from "@/lib/scroll-table";
import { cn } from "@/lib/utils";
import { useViewer } from "@/components/rbac/viewer-context";
import { Button } from "@/components/ui/button";

type ListResponse<T> = { items?: T[]; next_cursor?: string; error?: { message?: string } };

export function AdminListPanel<T extends Record<string, unknown>>({
  path,
  title,
  columns,
  rowHref,
  onRowSelect,
  rowSelected,
  actions,
  emptyTitle = "暂无记录",
  emptyDetail = "当前条件下暂无记录，可调整筛选条件后重试。",
  stickyEnds = true,
  density = "admin",
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
  density?: ScrollTableDensity;
}) {
  const router = useRouter();
  const tc = useTranslations("common");
  const [q, setQ] = useState("");
  const [search, setSearch] = useState("");
  const [cursor, setCursor] = useState("");
  const [previous, setPrevious] = useState<string[]>([]);
  const viewer = useViewer();
  const stateKey = path.split("?")[0].replaceAll("/", "_");
  useEffect(() => {
    const restore = () => {
      const params = new URLSearchParams(window.location.search);
      const value = params.get(`${stateKey}_q`) || "";
      setQ(value); setSearch(value); setCursor(params.get(`${stateKey}_cursor`) || ""); setPrevious([]);
    };
    restore(); window.addEventListener("popstate", restore);
    return () => window.removeEventListener("popstate", restore);
  }, [stateKey]);
  function updateLocation(value: string, next: string) {
    const url = new URL(window.location.href);
    for (const [key, item] of [[`${stateKey}_q`, value], [`${stateKey}_cursor`, next]]) {
      if (item) url.searchParams.set(key, item); else url.searchParams.delete(key);
    }
    window.history.replaceState(null, "", url.toString());
  }
  const params = new URLSearchParams(path.split("?")[1]);
  if (search) params.set("q", search);
  if (cursor) params.set("cursor", cursor);
  const href = `${path.split("?")[0]}${params.size ? `?${params}` : ""}`;
  const query = useQuery({
    queryKey: [viewer.userId, href],
    queryFn: () => apiClient<ListResponse<T>>("GET", href),
  });
  const data = query.data?.items ?? [];
  const table = useReactTable({ data, columns, getCoreRowModel: getCoreRowModel() });
  const colCount = columns.length;

  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-lg font-semibold tracking-tight">{title}</h2>
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          {actions}
          <form className="flex items-center gap-2" onSubmit={event => { event.preventDefault(); setSearch(q.trim()); setCursor(""); setPrevious([]); updateLocation(q.trim(), ""); }}><Input
            className="w-44 sm:w-56"
            aria-label={tc("filter")}
            placeholder={tc("filter")}
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
          <Button size="sm" variant="outline" type="submit">{tc("search")}</Button></form>
        </div>
      </div>
      {query.isError || query.data?.error ? (
        <p className="mb-3 text-sm text-ink-secondary">{query.data?.error?.message || tc("listFailed")}</p>
      ) : null}
      <div className="overflow-x-auto">
        <table className={cn("w-full text-left text-sm", colCount > 7 ? "min-w-[52rem]" : colCount > 4 ? "min-w-[34rem]" : "")}>
          <thead>
            {table.getHeaderGroups().map((group) => (
              <tr key={group.id} className="group border-b border-hairline">
                {group.headers.map((header, index) => (
                  <th key={header.id} className={stickyColumnClass(index, colCount, { stickyEnds, density, header: true })}>
                    {flexRender(header.column.columnDef.header, header.getContext())}
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody>
            {query.isLoading ? (
              <tr><td colSpan={Math.max(colCount, 1)} className="px-3 py-6"><p role="status">{tc("listLoading")}</p></td></tr>
            ) : data.length === 0 ? (
              <tr>
                <td colSpan={Math.max(colCount, 1)} className="px-3 py-6">
                  <EmptyState
                    title={query.isError || query.data?.error ? tc("listFailed") : emptyTitle}
                    detail={query.isError || query.data?.error ? tc("listRetry") : emptyDetail}
                  />
                </td>
              </tr>
            ) : (
              table.getRowModel().rows.map((row) => (
                <tr
                  key={row.id}
                  data-selected={rowSelected?.(row.original) ? "true" : undefined}
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
                    <td key={cell.id} className={stickyColumnClass(index, colCount, { stickyEnds, density })}>
                      {index === 0 && rowHref ? <Link href={rowHref(row.original)} onClick={event => event.stopPropagation()} className="text-brand-emphasis underline underline-offset-2">{flexRender(cell.column.columnDef.cell, cell.getContext())}</Link> : index === 0 && onRowSelect ? <Button size="sm" variant="ghost" onClick={event => { event.stopPropagation(); onRowSelect(row.original); }}>{tc("open")} · {flexRender(cell.column.columnDef.cell, cell.getContext())}</Button> : flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </td>
                  ))}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
      <div className="mt-4 flex flex-wrap items-center gap-3 text-sm">
        {cursor ? <Button size="sm" variant="outline" disabled={query.isFetching} onClick={() => { const next = previous.at(-1) || ""; setPrevious(items => items.slice(0, -1)); setCursor(next); updateLocation(search, next); }}>{tc(previous.length ? "previousPage" : "firstPage")}</Button> : null}
        <span>{query.isFetching ? tc("listLoading") : tc("pageCount", { count: data.length })}</span>
        {query.data?.next_cursor ? <Button size="sm" variant="outline" disabled={query.isFetching} onClick={() => { const next = query.data?.next_cursor || ""; setPrevious(items => [...items, cursor]); setCursor(next); updateLocation(search, next); }}>{tc("nextPage")}</Button> : null}
      </div>
    </section>
  );
}
