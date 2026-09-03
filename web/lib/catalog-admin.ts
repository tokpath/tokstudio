export function providerKindLabel(kind?: string): string {
  switch ((kind || "").trim().toLowerCase()) {
    case "aggregator":
      return "聚合";
    case "direct":
      return "直连";
    default:
      return kind?.trim() || "—";
  }
}

export function healthTone(health?: string): "success" | "warn" | "neutral" {
  switch ((health || "").trim().toLowerCase()) {
    case "available":
      return "success";
    case "degraded":
    case "unavailable":
    case "maintenance":
      return "warn";
    default:
      return "neutral";
  }
}

export function catalogStatusTone(status?: string): "success" | "warn" | "neutral" {
  switch ((status || "").trim().toLowerCase()) {
    case "active":
    case "published":
      return "success";
    case "maintenance":
    case "degraded":
    case "draft":
    case "reviewed":
    case "disabled":
    case "deprecated":
    case "rejected":
      return "warn";
    default:
      return "neutral";
  }
}

export function statusWord(value?: string): string {
  const text = (value || "").trim();
  return text ? text.toUpperCase() : "—";
}

export function syncStateLabel(state?: string): string {
  switch ((state || "").trim().toLowerCase()) {
    case "draft":
      return "待审核";
    case "reviewed":
      return "已通过";
    case "rejected":
      return "已拒绝";
    case "published":
      return "已发布";
    default:
      return state?.trim() || "—";
  }
}

export function formatRpm(limit?: number): string {
  if (!limit || limit <= 0) {
    return "未限制";
  }
  return String(limit);
}

export function formatCredentialRef(ref?: string): string {
  const text = (ref || "").trim();
  return text || "未配置";
}

export function formatProviderSlugs(providers?: string[]): string {
  const items = (providers || []).map((item) => item.trim()).filter(Boolean);
  return items.length > 0 ? items.join(" · ") : "未挂载";
}
