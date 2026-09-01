"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { apiBase } from "@/lib/api";
import { type DimMoney, type UsageEvent, dimToKeyBuckets, shortKeyRef, usageTokens } from "@/lib/usage";

type Usage = {
  usage_minor?: number;
  prompt_tokens?: number;
  completion_tokens?: number;
  video_seconds?: number;
  image_count?: number;
};

export default function ChannelUsage() {
  const t = useTranslations("channelUi");
  const [usage, setUsage] = useState<Usage>({});
  const [keys, setKeys] = useState<DimMoney[]>([]);
  const [items, setItems] = useState<UsageEvent[]>([]);
  const [message, setMessage] = useState(t("usageHint"));

  async function refresh() {
    const response = await fetch(`${apiBase}/channel/usage`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || t("needAdmin"));
      return;
    }
    setUsage((body.usage || {}) as Usage);
    setKeys(body.keys || []);
    setItems(body.items || []);
    setMessage(t("usageDone"));
  }

  const byKey = dimToKeyBuckets(keys);

  return (
    <Card>
      <CardTitle className="mb-3 text-xl font-medium">{t("usageTitle")}</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">{t("usageLead")}</p>
      <Button variant="outline" onClick={refresh}>
        {t("refreshUsage")}
      </Button>
      <section className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4" aria-label={t("usageTitle")}>
        {[
          { t: t("wholesale"), v: String(usage.usage_minor ?? 0), d: "micro-USD" },
          { t: "Prompt", v: String(usage.prompt_tokens ?? 0), d: "tokens" },
          { t: "Completion", v: String(usage.completion_tokens ?? 0), d: "tokens" },
          { t: t("media"), v: `${usage.video_seconds ?? 0}s / ${usage.image_count ?? 0}`, d: t("mediaHint") },
        ].map((card) => (
          <div key={card.t} className="rounded-card border border-hairline bg-canvas p-4">
            <p className="th-eyebrow text-ink-mute">{card.t}</p>
            <p className="mt-2 font-mono text-[22px] font-medium leading-none tabular-nums tracking-tight">{card.v}</p>
            <p className="mt-2 text-sm text-ink-secondary">{card.d}</p>
          </div>
        ))}
      </section>
      <h3 className="mt-5 mb-2 text-sm font-medium">{t("byApiKey")}</h3>
      <div className="overflow-x-auto rounded-card border border-hairline">
        <table className="w-full text-left text-sm">
          <thead className="bg-canvas text-ink-mute">
            <tr>
              <th className="px-3 py-2 font-medium">{t("colApiKey")}</th>
              <th className="px-3 py-2 font-medium">{t("colPrompt")}</th>
              <th className="px-3 py-2 font-medium">{t("colCompletion")}</th>
              <th className="px-3 py-2 font-medium">{t("colAmount")}</th>
            </tr>
          </thead>
          <tbody>
            {byKey.length === 0 ? (
              <tr>
                <td className="px-3 py-3 text-ink-secondary" colSpan={4}>
                  {message}
                </td>
              </tr>
            ) : (
              byKey.map((row) => (
                <tr key={row.api_key_id || "none"} className="border-t border-hairline">
                  <td className="px-3 py-2 font-mono text-xs">{shortKeyRef(row.api_key_id)}</td>
                  <td className="px-3 py-2 font-mono tabular-nums">{row.prompt}</td>
                  <td className="px-3 py-2 font-mono tabular-nums">{row.completion}</td>
                  <td className="px-3 py-2 font-mono tabular-nums">{row.amount}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
      {items.length > 0 ? (
        <div className="mt-4 overflow-x-auto rounded-card border border-hairline">
          <table className="w-full text-left text-sm">
            <thead className="bg-canvas text-ink-mute">
              <tr>
                <th className="px-3 py-2 font-medium">{t("colApiKey")}</th>
                <th className="px-3 py-2 font-medium">{t("colModel")}</th>
                <th className="px-3 py-2 font-medium">{t("colPrompt")}</th>
                <th className="px-3 py-2 font-medium">{t("colCompletion")}</th>
                <th className="px-3 py-2 font-medium">{t("colAmount")}</th>
              </tr>
            </thead>
            <tbody>
              {items.map((row) => {
                const tokens = usageTokens(row);
                return (
                  <tr key={row.id} className="border-t border-hairline">
                    <td className="px-3 py-2 font-mono text-xs">{shortKeyRef(row.api_key_id)}</td>
                    <td className="px-3 py-2 font-mono text-xs">{row.public_model_id || "—"}</td>
                    <td className="px-3 py-2 font-mono tabular-nums">{tokens.prompt}</td>
                    <td className="px-3 py-2 font-mono tabular-nums">{tokens.completion}</td>
                    <td className="px-3 py-2 font-mono tabular-nums">{row.customer_amount_minor ?? "—"}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : null}
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
