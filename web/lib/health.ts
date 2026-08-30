export const HEALTH_FIELD_ORDER = ["status", "service", "version", "request_id"] as const;

export const HEALTH_FIELD_LABELS: Record<string, string> = {
  status: "Status",
  service: "Service",
  version: "Version",
  request_id: "Request",
};

export function listHealthFields(
  health: Record<string, string> | { error: string },
): { key: string; label: string; value: string }[] {
  if ("error" in health) {
    return [{ key: "api", label: "API", value: "unreachable" }];
  }
  const keys: string[] = [...HEALTH_FIELD_ORDER];
  for (const key of Object.keys(health)) {
    if (!keys.includes(key)) {
      keys.push(key);
    }
  }
  return keys
    .filter((key) => health[key] !== undefined)
    .map((key) => ({
      key,
      label: HEALTH_FIELD_LABELS[key] ?? key,
      value: health[key],
    }));
}

/** 公共站示例只探活，不内嵌真实 Key。 */
export function formatHealthCurl(apiBase: string): string {
  return `curl -sS ${apiBase.replace(/\/$/, "")}/healthz`;
}
