"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { UsageCharts } from "@/components/usage-charts";
import { apiBase } from "@/lib/api";
import {
  type APIKeyOption,
  type DimMoney,
  type UsageEvent,
  bucketsToMetricPoints,
  dimToKeyBuckets,
  filterUsage,
  keyLabel,
  summarizeUsage,
} from "@/lib/usage";

type LedgerRow = {
  id: string;
  event_type?: string;
  amount_minor?: number;
};

const selectClass = "h-10 min-w-[12rem] rounded-control border border-hairline bg-canvas-raised px-3 text-sm";

/** 用量汇总：指标 + 图表 + 按 Key 汇总。逐条回单在 /app/activity（docs/14）。 */
export default function UsagePanel() {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const tChart = useTranslations("charts");
  const [usage, setUsage] = useState<UsageEvent[]>([]);
  const [keysDim, setKeysDim] = useState<DimMoney[]>([]);
  const [modelDims, setModelDims] = useState<DimMoney[]>([]);
  const [ledger, setLedger] = useState<LedgerRow[]>([]);
  const [keys, setKeys] = useState<APIKeyOption[]>([]);
  const [keyFilter, setKeyFilter] = useState("");
  const [modelFilter, setModelFilter] = useState("");
  const [message, setMessage] = useState(t("usageHint"));

  async function refresh(nextKey = keyFilter, nextModel = modelFilter) {
    const params = new URLSearchParams();
    if (nextKey) params.set("api_key_id", nextKey);
    if (nextModel) params.set("public_model_id", nextModel);
    params.set("limit", "100");
    const qs = params.toString();
    const [usageRes, ledgerRes, keysRes] = await Promise.all([
      fetch(`${apiBase}/v1/me/usage?${qs}`, { credentials: "include" }),
      fetch(`${apiBase}/v1/me/ledger`, { credentials: "include" }),
      fetch(`${apiBase}/v1/me/api-keys`, { credentials: "include" }),
    ]);
    const usageBody = await usageRes.json();
    const ledgerBody = await ledgerRes.json();
    const keysBody = await keysRes.json();
    if (!usageRes.ok) {
      setMessage(usageBody.error?.message || tc("notLoggedIn"));
      return;
    }
    setUsage(usageBody.items || []);
    setKeysDim(usageBody.keys || []);
    setModelDims(usageBody.models || []);
    setLedger(ledgerBody.items || []);
    setKeys(keysBody.items || []);
    setMessage(t("usageDone"));
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const filtered = useMemo(() => filterUsage(usage, { apiKeyId: keyFilter, model: modelFilter }), [usage, keyFilter, modelFilter]);
  const summary = summarizeUsage(filtered);
  const models = modelDims.map((row) => row.key).filter(Boolean);
  const byKey = keyFilter ? dimToKeyBuckets(keysDim).filter((row) => row.api_key_id === keyFilter) : dimToKeyBuckets(keysDim);

  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold tracking-tight">{t("usageTitle")}</h2>
          <p className="mt-2 text-sm text-ink-secondary">{t("usageLead")}</p>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link href="/app/activity">{t("usageToActivity")}</Link>
        </Button>
      </div>
      <div className="mb-5 flex flex-wrap items-end gap-4">
        <label className="flex flex-col gap-1.5 text-sm">
          <span className="text-ink-mute">{t("filterApiKey")}</span>
          <select
            className={selectClass}
            aria-label={t("filterApiKey")}
            value={keyFilter}
            onChange={(e) => {
              const value = e.target.value;
              setKeyFilter(value);
              void refresh(value, modelFilter);
            }}
          >
            <option value="">{t("allApiKeys")}</option>
            {keys.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name || item.id}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1.5 text-sm">
          <span className="text-ink-mute">{t("filterModel")}</span>
          <select
            className={selectClass}
            aria-label={t("filterModel")}
            value={modelFilter}
            onChange={(e) => {
              const value = e.target.value;
              setModelFilter(value);
              void refresh(keyFilter, value);
            }}
          >
            <option value="">{t("allModels")}</option>
            {models.map((model) => (
              <option key={model} value={model}>
                {model}
              </option>
            ))}
          </select>
        </label>
        <Button type="button" variant="outline" onClick={() => void refresh()}>
          {t("usageRefresh")}
        </Button>
      </div>
      <section className="mb-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-4" aria-label={t("usageTitle")}>
        {[
          { k: t("statRequests"), v: String(summary.requests) },
          { k: t("statPrompt"), v: String(summary.prompt) },
          { k: t("statCompletion"), v: String(summary.completion) },
          { k: t("statSpend"), v: String(summary.amount) },
        ].map((card) => (
          <div key={card.k} className="rounded-card border border-hairline bg-canvas p-4">
            <p className="th-eyebrow text-ink-mute">{card.k}</p>
            <p className="mt-2 font-mono text-[22px] font-medium leading-none tabular-nums tracking-tight">{card.v}</p>
          </div>
        ))}
      </section>
      <UsageCharts
        events={filtered}
        breakdown={bucketsToMetricPoints(byKey, (id) => keyLabel(id, keys))}
        breakdownTitle={tChart("byKey")}
      />
      <h3 className="mb-2 text-sm font-medium">{t("byApiKey")}</h3>
      <div className="mb-4 overflow-x-auto rounded-card border border-hairline">
        <table className="w-full text-left text-sm">
          <thead className="bg-canvas text-ink-mute">
            <tr>
              <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colApiKey")}</th>
              <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("statRequests")}</th>
              <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colPrompt")}</th>
              <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colCompletion")}</th>
              <th className="th-eyebrow px-4 py-3 text-ink-mute">{t("colAmount")}</th>
            </tr>
          </thead>
          <tbody>
            {byKey.length === 0 ? (
              <tr>
                <td className="px-4 py-3.5 text-ink-secondary" colSpan={5}>
                  {t("usageEmpty")}
                </td>
              </tr>
            ) : (
              byKey.map((row) => (
                <tr key={row.api_key_id || "none"} className="border-t border-hairline">
                  <td className="px-4 py-3 font-mono text-xs">{keyLabel(row.api_key_id, keys)}</td>
                  <td className="px-4 py-3 font-mono tabular-nums">{row.requests}</td>
                  <td className="px-4 py-3 font-mono tabular-nums">{row.prompt}</td>
                  <td className="px-4 py-3 font-mono tabular-nums">{row.completion}</td>
                  <td className="px-4 py-3 font-mono tabular-nums">{row.amount}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
      <p className="mt-3 text-sm text-ink-secondary">
        {t("usageCount", { usage: filtered.length, ledger: ledger.length, message })}
      </p>
    </section>
  );
}
