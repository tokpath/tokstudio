/** 请求明细筛选：只写用户台 URL，再映射到 GET /v1/me/usage。 */

export const ACTIVITY_PATH = "/app/activity";
export const ACTIVITY_PAGE_SIZE = 100;

export const ACTIVITY_RANGES = ["", "today", "7d", "30d", "custom"] as const;
export type ActivityRange = (typeof ACTIVITY_RANGES)[number];

export const ACTIVITY_STATES = ["failed", "confirmed", "pending", "voided", "pending_reconciliation"] as const;

export type ActivityQuery = {
  range?: ActivityRange;
  from?: string;
  to?: string;
  model?: string;
  status?: string;
  key?: string;
};

type SearchLike = { get: (name: string) => string | null };

export function parseActivitySearchParams(search: SearchLike): ActivityQuery {
  const rangeRaw = (search.get("range") || "").trim();
  const range = (ACTIVITY_RANGES as readonly string[]).includes(rangeRaw) ? (rangeRaw as ActivityRange) : "";
  return {
    range: range || undefined,
    from: dateParam(search.get("from")),
    to: dateParam(search.get("to")),
    model: textParam(search.get("model")),
    status: textParam(search.get("status")),
    key: textParam(search.get("key")),
  };
}

export function activityHref(query: ActivityQuery = {}): string {
  const params = new URLSearchParams();
  if (query.range) params.set("range", query.range);
  if (query.from && query.range === "custom") params.set("from", query.from);
  if (query.to && query.range === "custom") params.set("to", query.to);
  if (query.model) params.set("model", query.model);
  if (query.status) params.set("status", query.status);
  if (query.key) params.set("key", query.key);
  const qs = params.toString();
  return qs ? `${ACTIVITY_PATH}?${qs}` : ACTIVITY_PATH;
}

export function hasActivityFilters(query: ActivityQuery): boolean {
  return Boolean(query.range || query.from || query.to || query.model || query.status || query.key);
}

/** 模型 / API Key 下拉选项：合并维度汇总与当前页，去空去重，避免筛完只剩当前这一项。 */
export function mergeFilterValues(...groups: Array<Iterable<string | undefined | null> | undefined>): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  for (const group of groups) {
    if (!group) {
      continue;
    }
    for (const raw of group) {
      const value = (raw || "").trim();
      if (!value || seen.has(value)) {
        continue;
      }
      seen.add(value);
      out.push(value);
    }
  }
  return out;
}

export function dimFilterKeys(rows?: Array<{ key?: string }> | null): string[] {
  return mergeFilterValues((rows || []).map((row) => row.key));
}

export function resolveActivityWindow(query: ActivityQuery, now = new Date()): { from?: string; to?: string } {
  if (query.range === "today") {
    return { from: isoDateUTC(now) };
  }
  if (query.range === "7d") {
    return { from: isoDateUTC(addUtcDays(now, -7)) };
  }
  if (query.range === "30d") {
    return { from: isoDateUTC(addUtcDays(now, -30)) };
  }
  if (query.range === "custom" || !query.range) {
    return { from: query.from, to: query.to };
  }
  return {};
}

export function activityApiQuery(query: ActivityQuery, now = new Date()): URLSearchParams {
  const params = new URLSearchParams();
  params.set("limit", String(ACTIVITY_PAGE_SIZE));
  const window = resolveActivityWindow(query, now);
  if (window.from) params.set("from", window.from);
  if (window.to) params.set("to", window.to);
  if (query.model) params.set("public_model_id", query.model);
  if (query.status) params.set("state", query.status);
  if (query.key) params.set("api_key_id", query.key);
  return params;
}

export function isoDateUTC(value: Date): string {
  const y = value.getUTCFullYear();
  const m = String(value.getUTCMonth() + 1).padStart(2, "0");
  const d = String(value.getUTCDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

function addUtcDays(value: Date, days: number): Date {
  return new Date(Date.UTC(value.getUTCFullYear(), value.getUTCMonth(), value.getUTCDate() + days));
}

function dateParam(raw: string | null): string | undefined {
  const value = (raw || "").trim();
  return /^\d{4}-\d{2}-\d{2}$/.test(value) ? value : undefined;
}

function textParam(raw: string | null): string | undefined {
  const value = (raw || "").trim();
  return value ? value : undefined;
}
