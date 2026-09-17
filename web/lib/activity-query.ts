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

export type ActivityTimeWindow = {
  from?: string;
  to?: string;
  error?: "invalid" | "order";
};

export function resolvedTimeZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
}

/** 页面日历日 → [本地日 00:00, 次日 00:00)，RFC3339 瞬间。与列表 toLocaleString 使用同一 timeZone。 */
export function activityTimeWindow(query: ActivityQuery, now = new Date(), timeZone = resolvedTimeZone()): ActivityTimeWindow {
  if (query.range === "custom") {
    return civilWindow(query.from, query.to, timeZone);
  }
  if (query.range === "today" || query.range === "7d" || query.range === "30d") {
    const today = calendarDateInZone(now, timeZone);
    if (!today) {
      return { error: "invalid" };
    }
    const daysBack = query.range === "today" ? 0 : query.range === "7d" ? 7 : 30;
    return civilWindow(addCivilDays(today, -daysBack), today, timeZone);
  }
  if (query.from || query.to) {
    return civilWindow(query.from, query.to, timeZone);
  }
  return {};
}

export function resolveActivityWindow(query: ActivityQuery, now = new Date(), timeZone = resolvedTimeZone()): { from?: string; to?: string } {
  const window = activityTimeWindow(query, now, timeZone);
  if (window.error) {
    return {};
  }
  return { from: window.from, to: window.to };
}

export function activityApiQuery(query: ActivityQuery, now = new Date(), timeZone = resolvedTimeZone()): URLSearchParams {
  const params = new URLSearchParams();
  params.set("limit", String(ACTIVITY_PAGE_SIZE));
  const window = activityTimeWindow(query, now, timeZone);
  if (!window.error) {
    if (window.from) params.set("from", window.from);
    if (window.to) params.set("to", window.to);
  }
  if (query.model) params.set("public_model_id", query.model);
  if (query.result) params.set("result", query.result);
  if (query.billing) params.set("billing_state", query.billing);
  if (query.key) params.set("api_key_id", query.key);
  return params;
}

export function isInHalfOpen(at: Date, from?: string, to?: string): boolean {
  const t = at.getTime();
  if (from && t < Date.parse(from)) {
    return false;
  }
  if (to && t >= Date.parse(to)) {
    return false;
  }
  return true;
}

function civilWindow(fromDay: string | undefined, toDay: string | undefined, timeZone: string): ActivityTimeWindow {
  if ((fromDay && !isValidCivilDate(fromDay)) || (toDay && !isValidCivilDate(toDay))) {
    return { error: "invalid" };
  }
  const from = fromDay ? startOfZonedDay(fromDay, timeZone) : undefined;
  const to = toDay ? startOfZonedDay(addCivilDays(toDay, 1), timeZone) : undefined;
  if ((fromDay && !from) || (toDay && !to)) {
    return { error: "invalid" };
  }
  if (from && to && !(from.getTime() < to.getTime())) {
    return { error: "order" };
  }
  return { from: from?.toISOString(), to: to?.toISOString() };
}

export function calendarDateInZone(now: Date, timeZone: string): string | undefined {
  const parts = new Intl.DateTimeFormat("en-CA", { timeZone, year: "numeric", month: "2-digit", day: "2-digit" }).formatToParts(now);
  const year = parts.find((part) => part.type === "year")?.value;
  const month = parts.find((part) => part.type === "month")?.value;
  const day = parts.find((part) => part.type === "day")?.value;
  if (!year || !month || !day) {
    return undefined;
  }
  return `${year}-${month}-${day}`;
}

export function isValidCivilDate(ymd: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(ymd)) {
    return false;
  }
  const [year, month, day] = ymd.split("-").map(Number);
  const utc = new Date(Date.UTC(year, month - 1, day));
  return utc.getUTCFullYear() === year && utc.getUTCMonth() === month - 1 && utc.getUTCDate() === day;
}

function addCivilDays(ymd: string, days: number): string {
  const [year, month, day] = ymd.split("-").map(Number);
  const utc = new Date(Date.UTC(year, month - 1, day + days));
  return `${utc.getUTCFullYear()}-${String(utc.getUTCMonth() + 1).padStart(2, "0")}-${String(utc.getUTCDate()).padStart(2, "0")}`;
}

function startOfZonedDay(ymd: string, timeZone: string): Date | undefined {
  let guess = Date.parse(`${ymd}T00:00:00.000Z`);
  if (Number.isNaN(guess)) {
    return undefined;
  }
  for (let i = 0; i < 4; i += 1) {
    const offset = offsetAt(new Date(guess), timeZone);
    const next = Date.parse(`${ymd}T00:00:00.000Z`) - offset;
    if (next === guess) {
      break;
    }
    guess = next;
  }
  return new Date(guess);
}

function offsetAt(date: Date, timeZone: string): number {
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat("en-US", {
      timeZone,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      hourCycle: "h23",
    })
      .formatToParts(date)
      .filter((part) => part.type !== "literal")
      .map((part) => [part.type, part.value]),
  );
  const asUTC = Date.UTC(
    Number(parts.year),
    Number(parts.month) - 1,
    Number(parts.day),
    Number(parts.hour),
    Number(parts.minute),
    Number(parts.second),
  );
  return asUTC - date.getTime();
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
