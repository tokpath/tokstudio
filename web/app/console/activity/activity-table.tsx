"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { Button } from "@/components/ui/button";
import { apiBase } from "@/lib/api";
import { formatUsageTime, shortKeyRef, usageTokens } from "@/lib/usage";

type UsageRow = {
  id: string;
  request_id?: string;
  state?: string;
  customer_amount_minor?: number;
  public_model_id?: string;
  api_key_id?: string;
  prompt_tokens?: number;
  completion_tokens?: number;
  occurred_at?: string;
};

export function ActivityTable() {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const [rows, setRows] = useState<UsageRow[] | null>(null);
  const [message, setMessage] = useState(t("actHint"));

  async function refresh() {
    const response = await fetch(`${apiBase}/v1/me/usage`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setRows([]);
      setMessage(body.error?.message || tc("notLoggedIn"));
      return;
    }
    setRows(body.items || []);
    setMessage(t("actDone"));
  }

  useEffect(() => {
    void refresh();
    // 首次进入拉一次明细；refresh 闭包读当前文案即可。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (!rows || rows.length === 0) {
    return (
      <div className="flex flex-col gap-3">
        <Button variant="outline" size="sm" className="self-start" onClick={() => void refresh()}>
          {tc("refresh")}
        </Button>
        <EmptyLedger title={t("actEmpty")} detail={message} />
      </div>
    );
  }

  return (
    <div className="overflow-x-auto rounded-card border border-hairline">
      <table className="w-full text-left text-sm">
        <thead className="bg-canvas-raised text-ink-mute">
          <tr>
            <th className="px-4 py-3 font-medium">{t("colTime")}</th>
            <th className="px-4 py-3 font-medium">{t("colApiKey")}</th>
            <th className="px-4 py-3 font-medium">{t("colModel")}</th>
            <th className="px-4 py-3 font-medium">{t("colPrompt")}</th>
            <th className="px-4 py-3 font-medium">{t("colCompletion")}</th>
            <th className="px-4 py-3 font-medium">{t("colStatus")}</th>
            <th className="px-4 py-3 font-medium">{t("colReq")}</th>
            <th className="px-4 py-3 font-medium">{t("colAmount")}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => {
            const tokens = usageTokens(row);
            return (
              <tr key={row.id} className="border-t border-hairline">
                <td className="px-4 py-3 text-xs text-ink-mute">{formatUsageTime(row.occurred_at)}</td>
                <td className="px-4 py-3 font-mono text-xs">{shortKeyRef(row.api_key_id)}</td>
                <td className="px-4 py-3 font-mono text-xs">{row.public_model_id || "—"}</td>
                <td className="px-4 py-3 font-mono tabular-nums">{tokens.prompt}</td>
                <td className="px-4 py-3 font-mono tabular-nums">{tokens.completion}</td>
                <td className="px-4 py-3">{row.state || "—"}</td>
                <td className="px-4 py-3 font-mono text-xs text-ink-mute">{row.request_id || row.id}</td>
                <td className="px-4 py-3 font-mono tabular-nums">{row.customer_amount_minor ?? "—"}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
