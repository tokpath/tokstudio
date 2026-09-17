/** 请求明细筛选：只写用户台 URL，再映射到 GET /v1/me/requests。 */

export const ACTIVITY_PATH = "/app/activity";
export const ACTIVITY_PAGE_SIZE = 100;

export const ACTIVITY_RANGES = ["", "today", "7d", "30d", "custom"] as const;
export type ActivityRange = (typeof ACTIVITY_RANGES)[number];

export const ACTIVITY_RESULTS = ["succeeded", "failed"] as const;
export const ACTIVITY_BILLING_STATES = ["confirmed", "pending_reconciliation", "voided"] as const;

export type ActivityQuery = {
  range?: ActivityRange;
  from?: string;
  to?: string;
  model?: string;
  result?: string;
  billing?: string;
  key?: string;
};

export type ActivityRequest = {
  id: string;
  request_id?: string;
  public_model_id?: string;
  api_key_id?: string;
  result?: string;
  billing_state?: string;
  customer_amount_minor?: number;
  error_code?: string;
  http_status?: number;
  started_at?: string;
  prompt_tokens?: number;
  completion_tokens?: number;
  reasoning_tokens?: number;
};

type SearchLike = { get: (name: string) => string | null };

export function parseActivitySearchParams(search: SearchLike): ActivityQuery {
  const rangeRaw = (search.get("range") || "").trim();
  const range = (ACTIVITY_RANGES as readonly string[]).includes(rangeRaw) ? (rangeRaw as ActivityRange) : "";
  const legacy = textParam(search.get("status"));
  const result = textParam(search.get("result")) || (isActivityResult(legacy) ? legacy : undefined);
  const billing = textParam(search.get("billing")) || (isActivityBilling(legacy) ? legacy : undefined);
  return {
    range: range || undefined,
    from: dateParam(search.get("from")),
    to: dateParam(search.get("to")),
    model: textParam(search.get("model")),
    result,
    billing,
    key: textParam(search.get("key")),
  };
}

export function activityHref(query: ActivityQuery = {}): string {
  const params = new URLSearchParams();
  if (query.range) params.set("range", query.range);
  if (query.from && query.range === "custom") params.set("from", query.from);
  if (query.to && query.range === "custom") params.set("to", query.to);
  if (query.model) params.set("model", query.model);
  if (query.result) params.set("result", query.result);
  if (query.billing) params.set("billing", query.billing);
  if (query.key) params.set("key", query.key);
  const qs = params.toString();
  return qs ? `${ACTIVITY_PATH}?${qs}` : ACTIVITY_PATH;
}

export function hasActivityFilters(query: ActivityQuery): boolean {
  return Boolean(query.range || query.from || query.to || query.model || query.result || query.billing || query.key);
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
  if (query.result) params.set("result", query.result);
  if (query.billing) params.set("billing_state", query.billing);
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

function isActivityResult(value?: string): value is (typeof ACTIVITY_RESULTS)[number] {
  return Boolean(value && (ACTIVITY_RESULTS as readonly string[]).includes(value));
}

function isActivityBilling(value?: string): value is (typeof ACTIVITY_BILLING_STATES)[number] {
  return Boolean(value && (ACTIVITY_BILLING_STATES as readonly string[]).includes(value));
}
