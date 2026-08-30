export type Readyz = {
  status: string;
  checks?: Record<string, string>;
  request_id?: string;
};

export type CheckState = "ok" | "stale" | "error" | "unknown";
export type StatusTone = "success" | "hold" | "degraded" | "danger";
export type StatusWord = "READY" | "HOLD" | "DEGRADED" | "UNAVAILABLE";

export type StatusBadge = {
  word: StatusWord;
  tone: StatusTone;
};

/** 控制面检查的展示顺序，和 /readyz 字段对齐。 */
export const CHECK_ORDER = ["postgres", "redis", "migrations", "outbox_worker"] as const;

export const CHECK_LABELS: Record<string, string> = {
  postgres: "PostgreSQL",
  redis: "Redis",
  migrations: "Migrations",
  outbox_worker: "Outbox Worker",
};

export function summarizeReady(body: Readyz): string {
  if (body.status === "ready") {
    return "控制面已就绪：数据库、Redis 和迁移都通过了检查。";
  }
  const failed = Object.entries(body.checks ?? {})
    .filter(([, value]) => value !== "ok")
    .map(([key]) => key);
  return failed.length
    ? `还未就绪，失败项：${failed.join("、")}`
    : "服务还在启动，请稍后再看。";
}

export function isOperationalValue(value: string | undefined): boolean {
  return value === "ok" || value === "stale" || value === "error" || value === "unreachable" || value === "unknown";
}

export function classifyCheck(value: string | undefined): CheckState {
  if (value === "ok") return "ok";
  if (value === "stale") return "stale";
  if (value === "error" || value === "unreachable") return "error";
  if (value === "unknown") return "unknown";
  return "unknown";
}

export function badgeForCheck(state: CheckState): StatusBadge {
  if (state === "ok") return { word: "READY", tone: "success" };
  if (state === "stale") return { word: "DEGRADED", tone: "degraded" };
  if (state === "error") return { word: "UNAVAILABLE", tone: "danger" };
  return { word: "HOLD", tone: "hold" };
}

export function badgeForReady(body: Readyz | { error: string }): StatusBadge {
  if ("error" in body) return { word: "HOLD", tone: "hold" };
  if (body.status === "ready") return { word: "READY", tone: "success" };
  const states = Object.values(body.checks ?? {}).map(classifyCheck);
  if (states.includes("error")) return { word: "UNAVAILABLE", tone: "danger" };
  if (states.includes("stale")) return { word: "DEGRADED", tone: "degraded" };
  return { word: "HOLD", tone: "hold" };
}

/** 把就绪结果收成一张路由回单，对应 DESIGN.md 的可解释路径。 */
export function formatControlReceipt(body: Readyz | { error: string }): string {
  if ("error" in body) {
    return `control-plane → api → unreachable → HOLD → ${body.error}`;
  }
  const hops = CHECK_ORDER.map((key) => {
    const state = classifyCheck(body.checks?.[key]);
    return `${key}:${state}`;
  });
  const badge = badgeForReady(body);
  const request = body.request_id ? ` request ${body.request_id}` : "";
  return `control-plane → ${hops.join(" → ")} → ${badge.word}${request}`;
}

export function listChecks(body: Readyz | { error: string }): { key: string; label: string; value: string }[] {
  if ("error" in body) {
    return [{ key: "api", label: "API", value: "unreachable" }];
  }
  const keys: string[] = [...CHECK_ORDER];
  for (const key of Object.keys(body.checks ?? {})) {
    if (!keys.includes(key)) {
      keys.push(key);
    }
  }
  return keys.map((key) => ({
    key,
    label: CHECK_LABELS[key] ?? key,
    value: body.checks?.[key] ?? "unknown",
  }));
}
