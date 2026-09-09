"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { UsageCharts } from "@/components/usage-charts";
import { apiBase } from "@/lib/api";
import { type DimMoney, type UsageEvent, bucketsToMetricPoints, dimToKeyBuckets, shortKeyRef, usageTokens } from "@/lib/usage";

type Usage = {
  usage_minor?: number;
  prompt_tokens?: number;
  completion_tokens?: number;
  video_seconds?: number;
  image_count?: number;
};

export default function ChannelUsage() {
  const t = useTranslations("channelUi");
  const tChart = useTranslations("charts");
  const [usage, setUsage] = useState<Usage>({});
  const [keys, setKeys] = useState<DimMoney[]>([]);
  const [models, setModels] = useState<DimMoney[]>([]);
  const [items, setItems] = useState<UsageEvent[]>([]);
  const [keyFilter, setKeyFilter] = useState("");
  const [modelFilter, setModelFilter] = useState("");
  const [message, setMessage] = useState(t("usageHint"));

  async function refresh(nextKey = keyFilter, nextModel = modelFilter) {
    const params = new URLSearchParams();
    if (nextKey) params.set("api_key_id", nextKey);
    if (nextModel) params.set("public_model_id", nextModel);
    const qs = params.toString();
    const response = await fetch(`${apiBase}/channel/usage${qs ? `?${qs}` : ""}`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || t("needAdmin"));
      return;
    }
    setUsage((body.usage || {}) as Usage);
    setKeys(body.keys || []);
    setModels(body.models || []);
    setItems(body.items || []);
    setMessage(t("usageDone"));
  }

  const byKey = dimToKeyBuckets(keys);

  useEffect(() => {
    void refresh();
    // 进入页面拉一次真实 usage。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <Card>
      <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("usageTitle")}</CardTitle>
      <p className="mb-4 text-sm text-ink-secondary">{t("usageLead")}</p>
      <div className="mb-4 flex flex-wrap items-end gap-3">
        <label className="flex flex-col gap-1.5 text-sm">
          <span className="text-ink-mute">{t("filterApiKey")}</span>
          <select
            className="h-10 min-w-[12rem] rounded-control border border-hairline bg-canvas-raised px-3 text-sm"
            aria-label={t("filterApiKey")}
            value={keyFilter}
            onChange={(e) => {
              const value = e.target.value;
              setKeyFilter(value);
              void refresh(value, modelFilter);
            }}
          >
            <option value="">{t("allApiKeys")}</option>
            {keys.map((row) => (
              <option key={row.key} value={row.key}>
                {shortKeyRef(row.key)}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1.5 text-sm">
          <span className="text-ink-mute">{t("filterModel")}</span>
          <select
            className="h-10 min-w-[12rem] rounded-control border border-hairline bg-canvas-raised px-3 text-sm"
            aria-label={t("filterModel")}
            value={modelFilter}
            onChange={(e) => {
              const value = e.target.value;
              setModelFilter(value);
              void refresh(keyFilter, value);
            }}
          >
            <option value="">{t("allModels")}</option>
            {models.map((row) => (
              <option key={row.key} value={row.key}>
                {row.key}
              </option>
            ))}
          </select>
        </label>
        <Button variant="outline" onClick={() => void refresh()}>
          {t("refreshUsage")}
        </Button>
      </div>
      <section className="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-4" aria-label={t("usageTitle")}>
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
      <div className="mt-5">
        <UsageCharts events={items} breakdown={bucketsToMetricPoints(byKey, shortKeyRef)} breakdownTitle={tChart("byKey")} />
      </div>
      <h3 className="mb-2 text-sm font-medium">{t("byApiKey")}</h3>
      <div className="overflow-x-auto rounded-card border border-hairline">
        <table className="w-full text-left text-sm">
          <thead className="bg-canvas text-ink-mute">
            <tr>
              <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colApiKey")}</th>
              <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colPrompt")}</th>
              <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colCompletion")}</th>
              <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colAmount")}</th>
            </tr>
          </thead>
          <tbody>
            {byKey.length === 0 ? (
              <tr>
                <td className="px-4 py-3.5 text-ink-secondary" colSpan={4}>
                  {message}
                </td>
              </tr>
            ) : (
              byKey.map((row) => (
                <tr key={row.api_key_id || "none"} className="border-t border-hairline">
                  <td className="px-4 py-3 font-mono text-xs">{shortKeyRef(row.api_key_id)}</td>
                  <td className="px-4 py-3 font-mono tabular-nums">{row.prompt}</td>
                  <td className="px-4 py-3 font-mono tabular-nums">{row.completion}</td>
                  <td className="px-4 py-3 font-mono tabular-nums">{row.amount}</td>
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
                <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colApiKey")}</th>
                <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colModel")}</th>
                <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colPrompt")}</th>
                <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colCompletion")}</th>
                <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colAmount")}</th>
              </tr>
            </thead>
            <tbody>
              {items.map((row) => {
                const tokens = usageTokens(row);
                return (
                  <tr key={row.id} className="border-t border-hairline">
                    <td className="px-4 py-3 font-mono text-xs">{shortKeyRef(row.api_key_id)}</td>
                    <td className="px-4 py-3 font-mono text-xs">{row.public_model_id || "—"}</td>
                    <td className="px-4 py-3 font-mono tabular-nums">{tokens.prompt}</td>
                    <td className="px-4 py-3 font-mono tabular-nums">{tokens.completion}</td>
                    <td className="px-4 py-3 font-mono tabular-nums">{row.customer_amount_minor ?? "—"}</td>
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
