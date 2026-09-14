/** 接入提供商时可选的请求协议。Bifrost 是数据面，不作为选项。 */
export const PROVIDER_PROTOCOLS = [
  { value: "openai", label: "OpenAI 兼容" },
  { value: "anthropic", label: "Anthropic" },
  { value: "gemini", label: "Gemini" },
  { value: "ark", label: "火山方舟" },
  { value: "openrouter", label: "OpenRouter" },
] as const;

const LEGACY_ADAPTER_LABELS: Record<string, string> = {
  test: "沙箱回声",
  bifrost: "Bifrost",
};

export function protocolOptions(current?: string): { value: string; label: string }[] {
  const options = PROVIDER_PROTOCOLS.map((item) => ({ value: item.value, label: item.label }));
  const value = (current || "").trim().toLowerCase();
  if (value && !options.some((item) => item.value === value)) {
    return [{ value, label: adapterLabel(value) }, ...options];
  }
  return options;
}

export const PROVIDER_STATUSES = [
  { value: "active", label: "参与路由" },
  { value: "maintenance", label: "维护中（从路由拿掉）" },
  { value: "disabled", label: "已停用" },
] as const;

/** 对齐 catalog.applyStrategy：priority / weight / price / health。 */
export const ROUTE_STRATEGIES = [
  { value: "priority", label: "priority · 按优先级" },
  { value: "weight", label: "weight · 按权重" },
  { value: "price", label: "price · 按成本价" },
  { value: "health", label: "health · 按健康度" },
] as const;

/** 网关 ResolveRoute 只取 status=active 的路由组。 */
export const ROUTE_STATUSES = [
  { value: "active", label: "active · 参与选路" },
  { value: "inactive", label: "inactive · 停用" },
] as const;

function withCurrentOption(
  items: readonly { value: string; label: string }[],
  current?: string,
): { value: string; label: string }[] {
  const options = items.map((item) => ({ value: item.value, label: item.label }));
  const value = (current || "").trim().toLowerCase();
  if (value && !options.some((item) => item.value === value)) {
    return [{ value, label: value }, ...options];
  }
  return options;
}

export function routeStrategyOptions(current?: string): { value: string; label: string }[] {
  return withCurrentOption(ROUTE_STRATEGIES, current);
}

export function routeStatusOptions(current?: string): { value: string; label: string }[] {
  return withCurrentOption(ROUTE_STATUSES, current);
}

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
  if (!value) {
    return "—";
  }
  const protocol = PROVIDER_PROTOCOLS.find((item) => item.value === value);
  if (protocol) {
    return protocol.label;
  }
  return LEGACY_ADAPTER_LABELS[value] ?? (adapter?.trim() || "—");
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
  return ids.length > 0 ? ids.join(" · ") : "尚未关联模型";
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

/** 详情页上架按钮：对齐 ReviewModel / PublishModel / DeprecateModel 的拒绝条件。 */
export function modelLifecycleEnabled(status?: string, syncState?: string): {
  approve: boolean;
  reject: boolean;
  publish: boolean;
  deprecate: boolean;
} {
  const st = (status || "").trim().toLowerCase();
  const sync = (syncState || "").trim().toLowerCase();
  if (!st && !sync) {
    return { approve: false, reject: false, publish: false, deprecate: false };
  }
  const published = st === "published";
  const deprecated = st === "deprecated";
  const reviewed = sync === "reviewed";
  const rejected = sync === "rejected";
  const awaitingReview = !sync || sync === "draft";
  return {
    approve: !published && (awaitingReview || rejected || deprecated),
    reject: !published && !deprecated && (awaitingReview || reviewed),
    publish: !published && reviewed,
    deprecate: published,
  };
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
  return (ref || "").trim() ? "已配置" : "尚未配置";
}

export function formatProviderSlugs(providers?: string[]): string {
  const items = (providers || []).map((item) => item.trim()).filter(Boolean);
  return items.length > 0 ? items.join(" · ") : "尚未关联";
}

export type ProviderOption = { id: string; name: string; slug: string };

export function filterProviderOptions(items: ProviderOption[], q: string, limit = 50): ProviderOption[] {
  const needle = q.trim().toLowerCase();
  const matched = needle
    ? items.filter((item) => [item.slug, item.name, item.id].some((part) => String(part || "").toLowerCase().includes(needle)))
    : items;
  return matched.slice(0, limit);
}
