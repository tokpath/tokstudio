export type UsageEvent = {
  id: string;
  request_id?: string;
  user_id?: string;
  api_key_id?: string;
  public_model_id?: string;
  provider_id?: string;
  prompt_tokens?: number;
  completion_tokens?: number;
  reasoning_tokens?: number;
  customer_amount_minor?: number;
  wholesale_amount_minor?: number;
  state?: string;
  occurred_at?: string;
  unit_usage?: {
    prompt_tokens?: number;
    completion_tokens?: number;
    reasoning_tokens?: number;
  };
};

export type DimMoney = {
  dimension?: string;
  key: string;
  requests?: number;
  usage_minor?: number;
  revenue_minor?: number;
  prompt_tokens?: number;
  completion_tokens?: number;
  reasoning_tokens?: number;
};

export type APIKeyOption = { id: string; name: string; prefix?: string };

export type UsageSummary = {
  requests: number;
  prompt: number;
  completion: number;
  reasoning: number;
  amount: number;
};

export type KeyBucket = UsageSummary & { api_key_id: string };

export function usageTokens(row: UsageEvent) {
  const usage = row.unit_usage || {};
  return {
    prompt: row.prompt_tokens ?? usage.prompt_tokens ?? 0,
    completion: row.completion_tokens ?? usage.completion_tokens ?? 0,
    reasoning: row.reasoning_tokens ?? usage.reasoning_tokens ?? 0,
  };
}

// Keep real request/token counts, but only confirmed consumption contributes to spend.
// Older projections without a state retain their historical amount semantics.
export function usageSpend(row: UsageEvent, wholesaleFallback = false) {
  if (row.state && row.state !== "confirmed") return 0;
  return row.customer_amount_minor ?? (wholesaleFallback ? row.wholesale_amount_minor : 0) ?? 0;
}

export function summarizeUsage(rows: UsageEvent[] = []): UsageSummary {
  return rows.reduce<UsageSummary>(
    (acc, row) => {
      const tokens = usageTokens(row);
      acc.requests += 1;
      acc.prompt += tokens.prompt;
      acc.completion += tokens.completion;
      acc.reasoning += tokens.reasoning;
      acc.amount += usageSpend(row);
      return acc;
    },
    { requests: 0, prompt: 0, completion: 0, reasoning: 0, amount: 0 },
  );
}

export function groupUsageByAPIKey(rows: UsageEvent[] = []): KeyBucket[] {
  const map = new Map<string, KeyBucket>();
  for (const row of rows) {
    const id = row.api_key_id || "";
    const cur = map.get(id) ?? { api_key_id: id, requests: 0, prompt: 0, completion: 0, reasoning: 0, amount: 0 };
    const tokens = usageTokens(row);
    cur.requests += 1;
    cur.prompt += tokens.prompt;
    cur.completion += tokens.completion;
    cur.reasoning += tokens.reasoning;
    cur.amount += usageSpend(row);
    map.set(id, cur);
  }
  return [...map.values()].sort((a, b) => b.amount - a.amount || b.requests - a.requests);
}

export function filterUsage(rows: UsageEvent[] = [], filters: { apiKeyId?: string; model?: string } = {}) {
  return rows.filter((row) => {
    if (filters.apiKeyId && row.api_key_id !== filters.apiKeyId) {
      return false;
    }
    if (filters.model && row.public_model_id !== filters.model) {
      return false;
    }
    return true;
  });
}

export function uniqueModels(rows: UsageEvent[] = []) {
  return [...new Set(rows.map((row) => row.public_model_id).filter(Boolean))] as string[];
}

export function shortKeyRef(id?: string) {
  if (!id) {
    return "—";
  }
  return id.length > 14 ? `${id.slice(0, 12)}…` : id;
}

export function keyLabel(id: string | undefined, keys: APIKeyOption[] = []) {
  if (!id) {
    return "—";
  }
  const found = keys.find((item) => item.id === id);
  if (found?.name) {
    return found.name;
  }
  return shortKeyRef(id);
}

export function formatUsageTime(value?: string, timeZone?: string) {
  if (!value) {
    return "—";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString(undefined, timeZone ? { timeZone } : undefined);
}

export function dimToKeyBuckets(rows: DimMoney[] = []): KeyBucket[] {
  return rows
    .map((row) => ({
      api_key_id: row.key,
      requests: row.requests ?? 0,
      prompt: row.prompt_tokens ?? 0,
      completion: row.completion_tokens ?? 0,
      reasoning: row.reasoning_tokens ?? 0,
      amount: row.revenue_minor ?? row.usage_minor ?? 0,
    }))
    .sort((a, b) => b.amount - a.amount || b.requests - a.requests);
}

export function usageDayKey(value?: string) {
  if (!value) {
    return "";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value.slice(0, 10);
  }
  return parsed.toISOString().slice(0, 10);
}

export function groupUsageByDay(rows: UsageEvent[] = []) {
  const map = new Map<string, { day: string; requests: number; revenue_minor: number }>();
  for (const row of rows) {
    const day = usageDayKey(row.occurred_at) || "—";
    const cur = map.get(day) ?? { day, requests: 0, revenue_minor: 0 };
    cur.requests += 1;
    cur.revenue_minor += usageSpend(row, true);
    map.set(day, cur);
  }
  return [...map.values()].sort((a, b) => a.day.localeCompare(b.day));
}

export function groupUsageByModel(rows: UsageEvent[] = []) {
  const map = new Map<string, { key: string; requests: number; revenue_minor: number }>();
  for (const row of rows) {
    const key = row.public_model_id || "—";
    const cur = map.get(key) ?? { key, requests: 0, revenue_minor: 0 };
    cur.requests += 1;
    cur.revenue_minor += usageSpend(row, true);
    map.set(key, cur);
  }
  return [...map.values()].sort((a, b) => b.revenue_minor - a.revenue_minor || b.requests - a.requests);
}

export function bucketsToMetricPoints(
  rows: { api_key_id: string; requests: number; amount: number }[],
  labelFor = (id: string) => id || "—",
) {
  return rows.map((row) => ({
    key: labelFor(row.api_key_id),
    requests: row.requests,
    revenue_minor: row.amount,
  }));
}
