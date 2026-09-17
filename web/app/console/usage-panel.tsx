"use client";

import Link from "next/link";
import { useCallback, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { ActionRow, LeadActions } from "@/components/console/action-row";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { ListResourceView } from "@/components/console/list-resource-view";
import { Button } from "@/components/ui/button";
import { ScrollTable } from "@/components/ui/scroll-table";
import { UsageCharts } from "@/components/usage-charts";
import { useListResource } from "@/hooks/use-list-resource";
import { activityHref } from "@/lib/activity-query";
import { apiBase } from "@/lib/api";
import { fetchListItems, type ListLoadResult } from "@/lib/list-resource";
import { formatUsdMinor } from "@/lib/money";
import {
  type APIKeyOption,
  type DimMoney,
  type KeyBucket,
  type UsageEvent,
  bucketsToMetricPoints,
  filterUsage,
  groupUsageByAPIKey,
  keyLabel,
  summarizeUsage,
  uniqueModels,
} from "@/lib/usage";

const selectClass = "h-10 min-w-[12rem] rounded-control border border-hairline bg-canvas-raised px-3 text-sm";

type UsageExtras = {
  modelDims: DimMoney[];
  keyItems: APIKeyOption[];
};

async function loadUsage(keyFilter: string, modelFilter: string): Promise<ListLoadResult<UsageEvent>> {
  const params = new URLSearchParams();
  if (keyFilter) params.set("api_key_id", keyFilter);
  if (modelFilter) params.set("public_model_id", modelFilter);
  params.set("limit", "100");
  try {
    const response = await fetch(`${apiBase}/v1/me/usage?${params.toString()}`, { credentials: "include" });
    const body: unknown = await response.json().catch(() => ({}));
    const record =
      body && typeof body === "object"
        ? (body as {
            items?: UsageEvent[];
            keys?: DimMoney[];
            models?: DimMoney[];
            error?: { message?: string; code?: string };
          })
        : {};
    if (!response.ok) {
      return { ok: false, status: response.status, items: [], message: record.error?.message, code: record.error?.code };
    }
    let keyItems: APIKeyOption[] = [];
    const keys = await fetchListItems<APIKeyOption>(`${apiBase}/v1/me/api-keys`);
    if (keys.ok) {
      keyItems = keys.items ?? [];
    }
    return {
      ok: true,
      status: response.status,
      items: Array.isArray(record.items) ? record.items : [],
      extras: {
        modelDims: Array.isArray(record.models) ? record.models : [],
        keyItems,
      } satisfies UsageExtras,
    };
  } catch {
    return { ok: false, network: true, items: [] };
  }
}

/** 用量汇总：指标 + 图表 + 按 Key 汇总。逐条回单在 /app/activity（docs/14）。 */
export default function UsagePanel() {
  const t = useTranslations("user");
  const tChart = useTranslations("charts");
  const [modelDims, setModelDims] = useState<DimMoney[]>([]);
  const [keys, setKeys] = useState<APIKeyOption[]>([]);
  const [keyFilter, setKeyFilter] = useState("");
  const [modelFilter, setModelFilter] = useState("");
  const load = useCallback(() => loadUsage(keyFilter, modelFilter), [keyFilter, modelFilter]);
  const list = useListResource<UsageEvent>({
    queryKey: `${keyFilter}|${modelFilter}`,
    load,
    onAccepted: (result) => {
      if (!result.ok) {
        return;
      }
      const extras = result.extras as UsageExtras | undefined;
      setModelDims(extras?.modelDims ?? []);
      if (extras?.keyItems) {
        setKeys(extras.keyItems);
      }
    },
  });

  const usage = list.snapshot.items;
  const filtered = useMemo(
    () => filterUsage(usage, { apiKeyId: keyFilter, model: modelFilter }),
    [usage, keyFilter, modelFilter],
  );
  const summary = summarizeUsage(filtered);
  const byKey = groupUsageByAPIKey(filtered);
  const models = useMemo(() => {
    const names = new Set(uniqueModels(usage));
    for (const row of modelDims) {
      if (row.key) {
        names.add(row.key);
      }
    }
    if (modelFilter) {
      names.add(modelFilter);
    }
    return [...names];
  }, [usage, modelDims, modelFilter]);
  const usageOk = list.snapshot.phase === "empty" || list.snapshot.phase === "ready" || list.snapshot.phase === "stale";

  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <LeadActions
        lead={<p className="text-sm text-ink-secondary">{t("usageLead")}</p>}
        actions={
          <Button asChild variant="outline" size="sm">
            <Link href={activityHref({ model: modelFilter || undefined, key: keyFilter || undefined })}>{t("usageToActivity")}</Link>
          </Button>
        }
      />
      <div className="mb-5 flex flex-wrap items-end gap-4">
        <label className="flex flex-col gap-1.5 text-sm">
          <span className="text-ink-mute">{t("filterApiKey")}</span>
          <select
            className={selectClass}
            aria-label={t("filterApiKey")}
            value={keyFilter}
            onChange={(e) => setKeyFilter(e.target.value)}
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
            onChange={(e) => setModelFilter(e.target.value)}
          >
            <option value="">{t("allModels")}</option>
            {models.map((model) => (
              <option key={model} value={model}>
                {model}
              </option>
            ))}
          </select>
        </label>
        <ActionRow>
          <Button type="button" variant="outline" onClick={() => void list.reload()}>
            {t("usageRefresh")}
          </Button>
        </ActionRow>
      </div>
      <p className="mb-4 text-sm text-ink-secondary">{t("usageScope")}</p>
      <ListResourceView
        snapshot={list.snapshot}
        emptyTitle={t("usageEmpty")}
        emptyDetail={t("usageEmptyDetail")}
        loadingTitle={t("usageTitle")}
        onRetry={() => void list.reload()}
        name="usage"
        passEmpty
      >
        <section className="mb-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-4" aria-label={t("usageTitle")}>
          {[
            { k: t("statRequests"), v: usageOk ? String(summary.requests) : "—", testId: "usage-stat-requests" },
            { k: t("statPrompt"), v: usageOk ? String(summary.prompt) : "—" },
            { k: t("statCompletion"), v: usageOk ? String(summary.completion) : "—" },
            { k: t("statSpend"), v: usageOk ? formatUsdMinor(summary.amount) : "—", testId: "usage-stat-spend" },
          ].map((card) => (
            <div key={card.k} className="rounded-card border border-hairline bg-canvas p-4">
              <p className="th-eyebrow text-ink-mute">{card.k}</p>
              <p
                className="mt-2 font-mono text-[22px] font-medium leading-none tabular-nums tracking-tight"
                data-testid={"testId" in card ? card.testId : undefined}
              >
                {card.v}
              </p>
            </div>
          ))}
        </section>
        <UsageCharts
          events={filtered}
          breakdown={bucketsToMetricPoints(byKey, (id) => keyLabel(id, keys))}
          breakdownTitle={tChart("byKey")}
        />
        <h3 className="mb-2 text-sm font-medium">{t("byApiKey")}</h3>
        <ScrollTable
          density="ledger"
          className="mb-4 rounded-card border border-hairline"
          minWidthClassName="min-w-[40rem]"
          getRowId={(row: KeyBucket) => row.api_key_id || "none"}
          rows={byKey}
          empty={<EmptyLedger title={t("usageEmpty")} detail={t("usageEmptyDetail")} />}
          columns={[
            {
              id: "key",
              header: t("colApiKey"),
              cell: (row) => <span className="font-mono text-xs">{keyLabel(row.api_key_id, keys)}</span>,
            },
            {
              id: "requests",
              header: t("statRequests"),
              cell: (row) => <span className="font-mono tabular-nums">{row.requests}</span>,
            },
            {
              id: "prompt",
              header: t("colPrompt"),
              cell: (row) => <span className="font-mono tabular-nums">{row.prompt}</span>,
            },
            {
              id: "completion",
              header: t("colCompletion"),
              cell: (row) => <span className="font-mono tabular-nums">{row.completion}</span>,
            },
            {
              id: "amount",
              header: t("colAmount"),
              cell: (row) => (
                <span className="font-mono tabular-nums" data-testid={`usage-key-amount-${row.api_key_id || "none"}`}>
                  {formatUsdMinor(row.amount)}
                </span>
              ),
            },
          ]}
        />
        <p className="mt-3 text-sm text-ink-secondary">{t("usageCount", { usage: filtered.length, message: t("usageDone") })}</p>
      </ListResourceView>
    </section>
  );
}
