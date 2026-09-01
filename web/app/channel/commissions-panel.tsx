"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { LedgerTable } from "@/components/console/ledger-table";
import { Button } from "@/components/ui/button";
import { apiBase } from "@/lib/api";

type Allocation = {
  id?: string;
  user_id?: string;
  granted_minor?: number;
  consumed_minor?: number;
  remaining_minor?: number;
  status?: string;
};

export default function ChannelCommissions() {
  const t = useTranslations("channelUi");
  const tc = useTranslations("common");
  const [message, setMessage] = useState(t("commHint"));
  const [quota, setQuota] = useState<string>("—");
  const [issued, setIssued] = useState<string>("—");
  const [consumed, setConsumed] = useState<string>("—");
  const [ratioBPS, setRatioBPS] = useState<string>("—");
  const [allocations, setAllocations] = useState<Allocation[]>([]);

  async function refresh() {
    const [q, c, a] = await Promise.all([
      fetch(`${apiBase}/channel/quota`, { credentials: "include" }),
      fetch(`${apiBase}/channel/commissions`, { credentials: "include" }),
      fetch(`${apiBase}/channel/allocations`, { credentials: "include" }),
    ]);
    const qBody = await q.json();
    const cBody = await c.json();
    const aBody = await a.json();
    if (!q.ok && !c.ok) {
      setMessage(qBody.error?.message || t("needAdmin"));
      return;
    }
    setQuota(qBody.quota?.available_minor ?? "—");
    setIssued(qBody.quota?.issued_minor ?? "—");
    setConsumed(qBody.quota?.consumed_minor ?? "—");
    setRatioBPS(qBody.quota?.issue_ratio_bps ?? "—");
    setAllocations(Array.isArray(aBody.items) ? aBody.items : []);
    setMessage(t("commCount", { comm: cBody.items?.length ?? 0, issued: aBody.items?.length ?? 0 }));
  }

  return (
    <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
      <h2 className="mb-3 text-xl font-medium">{t("commTitle")}</h2>
      <p className="mb-3 text-sm text-ink-secondary">{t("commLead", { quota })}</p>
      <p className="mb-3 text-sm text-ink-secondary">{t("commMeta", { ratio: ratioBPS, issued, consumed })}</p>
      <h3 className="mb-2 text-lg font-medium">{t("issuedTitle")}</h3>
      <LedgerTable
        columns={[t("colUser"), t("colGranted"), t("colUsed"), t("colLeft"), t("colStatus")]}
        emptyTitle={t("emptyIssued")}
        emptyDetail={t("emptyIssuedDetail")}
        rows={allocations.map((item) => ({
          key: item.id || item.user_id || "alloc",
          cells: [
            item.user_id || "—",
            String(item.granted_minor ?? 0),
            String(item.consumed_minor ?? 0),
            String(item.remaining_minor ?? 0),
            item.status || "—",
          ],
        }))}
      />
      <Button type="button" variant="outline" onClick={refresh}>
        {tc("refresh")}
      </Button>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </section>
  );
}
