export const MISSING_UPSTREAM = "缺上游元数据";
export const UPSTREAM_FACTS_TITLE = "上游事实";

export type UpstreamFacts = {
  provider?: string;
  model?: string;
  request_id?: string;
  attempt_id?: string;
  fact_source?: string;
};

const FAKE_UPSTREAM = ["openai", "gpt-4", "gpt-4o", "claude-3", "gemini-pro", "estimate", "invented"];

export function hasCompleteUpstreamFacts(facts: UpstreamFacts | null | undefined): boolean {
  if (!facts) {
    return false;
  }
  return Boolean(trim(facts.provider) && trim(facts.model) && trim(facts.request_id));
}

export function missingUpstreamLabel(): string {
  return MISSING_UPSTREAM;
}

export function truncateFact(value: string | undefined, max = 12): string {
  const text = trim(value);
  if (!text) {
    return "";
  }
  if (text.length <= max) {
    return text;
  }
  return `${text.slice(0, max)}…`;
}

export function badgeLabel(facts: UpstreamFacts | null | undefined): string {
  if (!hasCompleteUpstreamFacts(facts)) {
    return MISSING_UPSTREAM;
  }
  return [
    truncateFact(facts?.provider),
    truncateFact(facts?.model),
    truncateFact(facts?.request_id),
  ].join(" / ");
}

/** 只回已有字段。缺什么不填假值。 */
export function factsJSON(facts: UpstreamFacts | null | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  if (!facts) {
    return out;
  }
  put(out, "provider", facts.provider);
  put(out, "model", facts.model);
  put(out, "request_id", facts.request_id);
  put(out, "attempt_id", facts.attempt_id);
  put(out, "fact_source", facts.fact_source);
  return out;
}

export function fromDiffRow(row: {
  provider_id?: string;
  upstream_model_id?: string;
  request_id?: string;
  attempt_id?: string;
  fact_source?: string;
}): UpstreamFacts {
  return {
    provider: trim(row.provider_id) || undefined,
    model: trim(row.upstream_model_id) || undefined,
    request_id: trim(row.request_id) || undefined,
    attempt_id: trim(row.attempt_id) || undefined,
    fact_source: trim(row.fact_source) || undefined,
  };
}

export function forbidsFakeUpstream(label: string): boolean {
  const text = label.toLowerCase();
  return !FAKE_UPSTREAM.some((banned) => text.includes(banned));
}

export function isSandboxFact(facts: UpstreamFacts | null | undefined): boolean {
  const src = trim(facts?.fact_source);
  return src === "sandbox" || src === "echo";
}

function trim(value: string | undefined): string {
  return (value || "").trim();
}

function put(out: Record<string, string>, key: string, value: string | undefined) {
  const text = trim(value);
  if (text) {
    out[key] = text;
  }
}
