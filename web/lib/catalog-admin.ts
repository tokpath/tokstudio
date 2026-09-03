export const PROVIDER_ADAPTERS = [
  { value: "test", label: "沙箱回声" },
  { value: "openai", label: "OpenAI 兼容" },
  { value: "anthropic", label: "Anthropic" },
  { value: "gemini", label: "Gemini" },
  { value: "ark", label: "火山方舟" },
  { value: "openrouter", label: "OpenRouter" },
  { value: "bifrost", label: "Bifrost" },
] as const;

export const PROVIDER_STATUSES = [
  { value: "active", label: "参与路由" },
  { value: "maintenance", label: "维护中（从路由拿掉）" },
  { value: "disabled", label: "已停用" },
] as const;

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

export function adapterLabel(adapter?: string): string {
  const value = (adapter || "").trim().toLowerCase();
  return PROVIDER_ADAPTERS.find((item) => item.value === value)?.label ?? (adapter?.trim() || "—");
}

export function providerStatusLabel(status?: string): string {
  switch ((status || "").trim().toLowerCase()) {
    case "active":
      return "参与路由";
    case "maintenance":
      return "维护中";
    case "disabled":
      return "已停用";
    default:
      return status?.trim() || "—";
  }
}

export function healthLabel(health?: string): string {
  switch ((health || "").trim().toLowerCase()) {
    case "available":
      return "正常";
    case "degraded":
      return "降级";
    case "unavailable":
      return "不可用";
    case "maintenance":
      return "维护中";
    default:
      return health?.trim() || "—";
  }
}

export function providerHref(idOrSlug: string): string {
  return `/admin/providers/${encodeURIComponent(idOrSlug)}`;
}

export type MappedPublicModel = {
  public_id?: string;
  vendor?: string;
  display_name?: string;
  upstream_model_id?: string;
  status?: string;
};

export function formatMappedModels(models?: MappedPublicModel[]): string {
  const ids = (models || []).map((item) => (item.public_id || "").trim()).filter(Boolean);
  return ids.length > 0 ? ids.join(" · ") : "未挂模型";
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
  return (ref || "").trim() ? "已配置" : "未配置";
}

export function formatProviderSlugs(providers?: string[]): string {
  const items = (providers || []).map((item) => item.trim()).filter(Boolean);
  return items.length > 0 ? items.join(" · ") : "未挂载";
}
