"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { apiBase } from "@/lib/api";

type UsageRow = {
  id: string;
  request_id?: string;
  state?: string;
  customer_amount_minor?: number;
  public_model_id?: string;
};

type LedgerRow = {
  id: string;
  entry_type?: string;
  amount_minor?: number;
};

export default function UsagePanel() {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const [usage, setUsage] = useState<UsageRow[]>([]);
  const [ledger, setLedger] = useState<LedgerRow[]>([]);
  const [message, setMessage] = useState(t("usageHint"));

  async function refresh() {
    const [usageRes, ledgerRes] = await Promise.all([
      fetch(`${apiBase}/v1/me/usage`, { credentials: "include" }),
      fetch(`${apiBase}/v1/me/ledger`, { credentials: "include" }),
    ]);
    const usageBody = await usageRes.json();
    const ledgerBody = await ledgerRes.json();
    if (!usageRes.ok) {
      setMessage(usageBody.error?.message || tc("notLoggedIn"));
      return;
    }
    setUsage(usageBody.items || []);
    setLedger(ledgerBody.items || []);
    setMessage(t("usageDone"));
  }

  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6 ">
      <h2 className="mb-3 text-xl font-medium tracking-tight">{t("usageTitle")}</h2>
      <p className="mb-4 text-sm text-ink-secondary">{t("usageLead")}</p>
      <Button type="button" variant="outline" className="mb-4" onClick={refresh}>
        {t("usageRefresh")}
      </Button>
      <p className="text-sm text-ink-secondary">{t("usageCount", { usage: usage.length, ledger: ledger.length, message })}</p>
    </section>
  );
}
