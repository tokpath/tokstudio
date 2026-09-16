"use client";

import { useCallback, useMemo, useState, type ReactNode } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { ListResourceView } from "@/components/console/list-resource-view";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ScrollTable } from "@/components/ui/scroll-table";
import { useListResource } from "@/hooks/use-list-resource";
import {
  ACTIVITY_PAGE_SIZE,
  ACTIVITY_STATES,
  type ActivityQuery,
  type ActivityRange,
  activityApiQuery,
  activityHref,
  dimFilterKeys,
  hasActivityFilters,
  mergeFilterValues,
  parseActivitySearchParams,
} from "@/lib/activity-query";
import { apiBase } from "@/lib/api";
import type { ListLoadResult } from "@/lib/list-resource";
import { formatUsdMinor } from "@/lib/money";
import { statusLabelKey, statusTone } from "@/lib/status-copy";
import { copyText } from "@/lib/submit-result";
import { formatUsageTime, shortKeyRef, uniqueModels, usageTokens, type UsageEvent } from "@/lib/usage";

const selectClass = "h-10 min-w-[10rem] rounded-control border border-hairline bg-canvas-raised px-3 text-sm";

function useStatusText() {
  const tc = useTranslations("common");
  return (status?: string) => {
    const key = statusLabelKey(status);
    if (key) {
      return tc(key);
    }
    if (!status) {
      return "—";
    }
    return tc("stUnknown", { status });
  };
}

async function loadActivity(query: ActivityQuery): Promise<ListLoadResult<UsageEvent> & { models: string[]; keys: string[] }> {
  try {
    const response = await fetch(`${apiBase}/v1/me/usage?${activityApiQuery(query).toString()}`, { credentials: "include" });
    const body: unknown = await response.json().catch(() => ({}));
    const record =
      body && typeof body === "object"
        ? (body as {
            items?: UsageEvent[];
            models?: Array<{ key?: string }>;
            keys?: Array<{ key?: string }>;
            error?: { message?: string; code?: string };
          })
        : {};
    const items = Array.isArray(record.items) ? record.items : [];
    return {
      ok: response.ok,
      status: response.status,
      items,
      models: mergeFilterValues(dimFilterKeys(record.models), uniqueModels(items)),
      keys: mergeFilterValues(
        dimFilterKeys(record.keys),
        items.map((row) => row.api_key_id),
      ),
      message: record.error?.message,
      code: record.error?.code,
    };
  } catch {
    return { ok: false, network: true, items: [], models: [], keys: [] };
  }
}

/** 请求明细：默认列可定位失败；筛选写在 URL。 */
export function ActivityTable() {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const statusText = useStatusText();
  const router = useRouter();
  const searchParams = useSearchParams();
  const query = useMemo(() => parseActivitySearchParams(searchParams), [searchParams]);
  const [selected, setSelected] = useState<UsageEvent | null>(null);
  const [modelOptions, setModelOptions] = useState<string[]>([]);
  const [keyOptions, setKeyOptions] = useState<string[]>([]);
  const load = useCallback(async () => {
    const result = await loadActivity(query);
    if (result.ok) {
      setModelOptions((prev) => mergeFilterValues(prev, result.models, [query.model]));
      setKeyOptions((prev) => mergeFilterValues(prev, result.keys, [query.key]));
    }
    return result;
  }, [query]);
  const list = useListResource<UsageEvent>({
    queryKey: activityHref(query),
    load,
  });

  function write(next: ActivityQuery) {
    router.replace(activityHref(next), { scroll: false });
  }

  const models = mergeFilterValues(modelOptions, [query.model]);
  const keys = mergeFilterValues(keyOptions, [query.key]);
  const states: string[] = [...ACTIVITY_STATES];
  for (const row of list.snapshot.items) {
    if (row.state && !states.includes(row.state)) {
      states.push(row.state);
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm text-ink-secondary">{t("actScope", { n: ACTIVITY_PAGE_SIZE })}</p>
      <div className="flex flex-wrap items-end gap-3">
        <label className="flex flex-col gap-1.5 text-sm">
          <span className="text-ink-mute">{t("filterTime")}</span>
          <select
            className={selectClass}
            aria-label={t("filterTime")}
            value={query.range || ""}
            onChange={(e) => write({ ...query, range: (e.target.value || undefined) as ActivityRange | undefined, from: undefined, to: undefined })}
          >
            <option value="">{t("rangeAll")}</option>
            <option value="today">{t("rangeToday")}</option>
            <option value="7d">{t("range7d")}</option>
            <option value="30d">{t("range30d")}</option>
            <option value="custom">{t("rangeCustom")}</option>
          </select>
        </label>
        {query.range === "custom" ? (
          <>
            <label className="flex flex-col gap-1.5 text-sm">
              <span className="text-ink-mute">{t("fromDate")}</span>
              <input
                type="date"
                className={selectClass}
                aria-label={t("fromDate")}
                value={query.from || ""}
                onChange={(e) => write({ ...query, range: "custom", from: e.target.value || undefined })}
              />
            </label>
            <label className="flex flex-col gap-1.5 text-sm">
              <span className="text-ink-mute">{t("toDate")}</span>
              <input
                type="date"
                className={selectClass}
                aria-label={t("toDate")}
                value={query.to || ""}
                onChange={(e) => write({ ...query, range: "custom", to: e.target.value || undefined })}
              />
            </label>
          </>
        ) : null}
        <label className="flex flex-col gap-1.5 text-sm">
          <span className="text-ink-mute">{t("filterModel")}</span>
          <select
            className={selectClass}
            aria-label={t("filterModel")}
            value={query.model || ""}
            onChange={(e) => write({ ...query, model: e.target.value || undefined })}
          >
            <option value="">{t("allModels")}</option>
            {models.map((model) => (
              <option key={model} value={model}>
                {model}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1.5 text-sm">
          <span className="text-ink-mute">{t("filterStatus")}</span>
          <select
            className={selectClass}
            aria-label={t("filterStatus")}
            value={query.status || ""}
            onChange={(e) => write({ ...query, status: e.target.value || undefined })}
          >
            <option value="">{t("allStatuses")}</option>
            {states.map((state) => (
              <option key={state} value={state}>
                {statusText(state)}
              </option>
            ))}
          </select>
        </label>
        {keys.length > 0 || query.key ? (
          <label className="flex flex-col gap-1.5 text-sm">
            <span className="text-ink-mute">{t("filterApiKey")}</span>
            <select
              className={selectClass}
              aria-label={t("filterApiKey")}
              value={query.key || ""}
              onChange={(e) => write({ ...query, key: e.target.value || undefined })}
            >
              <option value="">{t("allApiKeys")}</option>
              {keys.map((id) => (
                <option key={id} value={id}>
                  {shortKeyRef(id)}
                </option>
              ))}
            </select>
          </label>
        ) : null}
        <Button variant="outline" size="sm" onClick={() => void list.reload()}>
          {tc("refresh")}
        </Button>
      </div>
      <ListResourceView
        snapshot={list.snapshot}
        loadingTitle={t("actLoading")}
        emptyTitle={hasActivityFilters(query) ? t("actEmptyFilter") : t("actEmpty")}
        emptyDetail={hasActivityFilters(query) ? t("actEmptyFilterDetail") : t("actEmptyDetail")}
        onRetry={() => void list.reload()}
      >
        <ScrollTable
          density="ledger"
          className="rounded-card border border-hairline"
          getRowId={(row) => row.id}
          rows={list.snapshot.items}
          columns={[
            {
              id: "time",
              header: t("colTime"),
              cell: (row) => <span className="text-xs text-ink-mute">{formatUsageTime(row.occurred_at)}</span>,
            },
            {
              id: "model",
              header: t("colModel"),
              cell: (row) => <span className="font-mono text-xs">{row.public_model_id || "—"}</span>,
            },
            {
              id: "result",
              header: t("colResult"),
              cell: (row) => <Badge tone={statusTone(row.state)}>{statusText(row.state)}</Badge>,
            },
            {
              id: "amount",
              header: t("colAmount"),
              cell: (row) => (
                <span className="font-mono tabular-nums">{formatUsdMinor(row.customer_amount_minor, tc("lessThanCent"))}</span>
              ),
            },
            {
              id: "details",
              header: t("viewDetails"),
              cell: (row) => (
                <Button type="button" size="sm" variant="outline" onClick={() => setSelected(row)}>
                  {t("viewDetails")}
                </Button>
              ),
            },
          ]}
        />
      </ListResourceView>
      <RequestDetailDialog row={selected} onClose={() => setSelected(null)} statusText={statusText} />
    </div>
  );
}

function RequestDetailDialog({
  row,
  onClose,
  statusText,
}: {
  row: UsageEvent | null;
  onClose: () => void;
  statusText: (status?: string) => string;
}) {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const [copied, setCopied] = useState(false);
  const tokens = row ? usageTokens(row) : { prompt: 0, completion: 0, reasoning: 0 };
  const requestId = row?.request_id || row?.id || "";
  const failed = statusTone(row?.state) === "warn";

  return (
    <Dialog open={Boolean(row)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("viewDetails")}</DialogTitle>
          <DialogDescription>{row?.public_model_id || t("colModel")}</DialogDescription>
        </DialogHeader>
        {row ? (
          <dl className="grid gap-3 text-sm">
            <Detail term={t("colTime")} value={formatUsageTime(row.occurred_at)} />
            <Detail term={t("colResult")} value={<Badge tone={statusTone(row.state)}>{statusText(row.state)}</Badge>} />
            <Detail term={t("colAmount")} value={formatUsdMinor(row.customer_amount_minor, tc("lessThanCent"))} />
            <Detail term={t("actPromptTokens")} value={String(tokens.prompt)} />
            <Detail term={t("actCompletionTokens")} value={String(tokens.completion)} />
            {tokens.reasoning ? <Detail term={t("actReasoningTokens")} value={String(tokens.reasoning)} /> : null}
            <Detail term={t("filterApiKey")} value={<span className="font-mono">{row.api_key_id || "—"}</span>} />
            <div className="flex flex-wrap items-center gap-2">
              <dt className="text-ink-mute">{tc("requestNo")}</dt>
              <dd className="font-mono text-ink">{requestId}</dd>
              <Button
                type="button"
                size="sm"
                variant="outline"
                aria-label={tc("copyRequest")}
                onClick={() => {
                  void copyText(requestId).then((ok) => setCopied(ok));
                }}
              >
                {copied ? tc("copied") : tc("copy")}
              </Button>
            </div>
            {failed ? <p className="text-sm text-ink-secondary">{t("actNoFailReason")}</p> : null}
          </dl>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function Detail({ term, value }: { term: string; value: ReactNode }) {
  return (
    <div className="flex flex-wrap items-baseline gap-2">
      <dt className="text-ink-mute">{term}</dt>
      <dd className="font-mono tabular-nums text-ink">{value}</dd>
    </div>
  );
}
