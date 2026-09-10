"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { Input } from "@/components/ui/input";
import { AdminH2 } from "@/components/admin-h2";
import { SealConfirm } from "@/components/seal-confirm";
import { IfCan } from "@/components/rbac/if-can";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders } from "@/lib/confirm";
import { gapKey, pendingListPath, type UsageGap } from "@/lib/reconciliation";

export default function AdminReconciliationPage() {
  const t = useTranslations("adminUi");
  const [status, setStatus] = useState("pending_reconciliation");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [selected, setSelected] = useState<UsageGap | null>(null);
  const [picked, setPicked] = useState<string[]>([]);
  const [message, setMessage] = useState(t("pendingLead"));

  const listPath = useMemo(() => pendingListPath({ status, from, to }), [status, from, to]);
  const listQuery = useQuery({
    queryKey: ["admin-pending", listPath],
    queryFn: () => apiClient<{ items?: UsageGap[] }>("GET", listPath),
  });

  async function resolve(ids: string[]) {
    const keys = ids.filter(Boolean);
    if (!keys.length) {
      setMessage(t("pendingNeedSelection"));
      return;
    }
    const res = await fetch(`${apiBase}/admin/usage/pending/resolve`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({ ids: keys }),
    });
    const body = await res.json();
    setMessage(res.ok ? t("pendingResolved", { n: keys.length }) : body.error?.message || t("pendingResolveFail"));
    setPicked([]);
    if (res.ok) {
      setSelected(null);
      await listQuery.refetch();
    }
  }

  async function openGap(row: UsageGap) {
    setSelected(row);
    const key = gapKey(row);
    if (!key) {
      return;
    }
    const res = await fetch(`${apiBase}/admin/usage/pending/${encodeURIComponent(key)}`, { credentials: "include" });
    const body = await res.json();
    if (res.ok && body.item) {
      setSelected(body.item as UsageGap);
    }
  }

  function togglePick(id: string, checked: boolean) {
    setPicked((cur) => (checked ? Array.from(new Set([...cur, id])) : cur.filter((item) => item !== id)));
  }

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <AdminH2 k="reconciliation" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">{t("pendingLead")}</p>
        <div className="mb-4 flex flex-wrap items-end gap-2">
          <label className="flex flex-col gap-1.5 text-sm">
            <span className="text-ink-mute">{t("filterStatus")}</span>
            <select
              className="h-10 min-w-[12rem] rounded-control border border-hairline bg-canvas-raised px-3 text-sm"
              aria-label={t("filterStatus")}
              value={status}
              onChange={(e) => setStatus(e.target.value)}
            >
              <option value="pending_reconciliation">{t("statusPending")}</option>
              <option value="voided">{t("statusResolved")}</option>
              <option value="all">{t("statusAll")}</option>
            </select>
          </label>
          <label className="flex flex-col gap-1.5 text-sm">
            <span className="text-ink-mute">{t("filterFrom")}</span>
            <Input type="date" className="w-40" value={from} onChange={(e) => setFrom(e.target.value)} aria-label={t("filterFrom")} />
          </label>
          <label className="flex flex-col gap-1.5 text-sm">
            <span className="text-ink-mute">{t("filterTo")}</span>
            <Input type="date" className="w-40" value={to} onChange={(e) => setTo(e.target.value)} aria-label={t("filterTo")} />
          </label>
          <IfCan action="usage.resolve">
            <SealConfirm
              size="sm"
              title={t("markResolvedTitle")}
              description={t("markResolvedDesc")}
              validate={() => picked.length > 0}
              onConfirm={() => resolve(picked)}
            >
              {t("markResolved")}
            </SealConfirm>
          </IfCan>
        </div>
        <p className="text-sm text-ink-secondary">{message}</p>
        <p className="mt-2 text-sm text-ink-mute">
          <Link href="/admin/usage" className="underline-offset-2 hover:underline">
            {t("pendingToReplay")}
          </Link>
        </p>
      </section>
      <AdminListPanel<UsageGap>
        path={listPath}
        title={t("pendingList")}
        emptyTitle={t("pendingEmpty")}
        emptyDetail={t("pendingEmptyDetail")}
        onRowSelect={(row) => void openGap(row)}
        rowSelected={(row) => gapKey(row) === gapKey(selected || {})}
        columns={[
          {
            id: "pick",
            header: t("colPick"),
            cell: ({ row }) => {
              const id = gapKey(row.original);
              return (
                <input
                  type="checkbox"
                  aria-label={t("colPick")}
                  checked={picked.includes(id)}
                  onChange={(e) => {
                    e.stopPropagation();
                    togglePick(id, e.target.checked);
                  }}
                  onClick={(e) => e.stopPropagation()}
                />
              );
            },
          },
          { accessorKey: "occurred_at", header: t("colTime") },
          { accessorKey: "request_id", header: "Request" },
          { accessorKey: "api_key_id", header: "API Key" },
          { accessorKey: "public_model_id", header: t("colModel") },
          { accessorKey: "channel_org_id", header: t("colChannel") },
          { accessorKey: "reserved_minor", header: t("colReserved") },
          { accessorKey: "state", header: t("colState") },
        ]}
      />
      {selected ? (
        <section className="rounded-card border border-hairline bg-canvas-raised p-6" data-testid="usage-gap-detail">
          <h3 className="mb-3 text-lg font-semibold tracking-tight">{t("gapDetail")}</h3>
          <dl className="grid gap-3 text-sm sm:grid-cols-2">
            <div>
              <dt className="text-ink-mute">request_id</dt>
              <dd className="font-mono">{selected.request_id || "—"}</dd>
            </div>
            <div>
              <dt className="text-ink-mute">{t("colState")}</dt>
              <dd className="font-mono">{selected.state || "—"}</dd>
            </div>
            <div>
              <dt className="text-ink-mute">{t("colAuthStatus")}</dt>
              <dd className="font-mono">{selected.auth_status || "—"}</dd>
            </div>
            <div>
              <dt className="text-ink-mute">{t("colReserved")}</dt>
              <dd className="font-mono tabular-nums">{selected.reserved_minor ?? 0}</dd>
            </div>
            <div>
              <dt className="text-ink-mute">API Key</dt>
              <dd className="font-mono">{selected.api_key_id || "—"}</dd>
            </div>
            <div>
              <dt className="text-ink-mute">{t("colModel")}</dt>
              <dd className="font-mono">{selected.public_model_id || "—"}</dd>
            </div>
          </dl>
          <p className="mt-4 text-sm text-hold">{selected.missing_usage ? t("gapMissing") : t("gapHasUsage")}</p>
          <IfCan action="usage.resolve">
            <div className="mt-4">
              <SealConfirm
                size="sm"
                title={t("markResolvedTitle")}
                description={t("markResolvedDesc")}
                onConfirm={() => resolve([gapKey(selected)])}
              >
                {t("markResolved")}
              </SealConfirm>
            </div>
          </IfCan>
        </section>
      ) : null}
    </AdminShell>
  );
}
