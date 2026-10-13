import { defaultCopy, type CopyTranslator } from "@/lib/copy";
/** 接入提供商时可选的请求协议。Bifrost 是数据面，不作为选项。 */
export function providerProtocols(tUI: CopyTranslator = defaultCopy) { return [
  { value: "openai", label: tUI("desktopUi.hintCline") },
  { value: "anthropic", label: "Anthropic" },
  { value: "gemini", label: "Gemini" },
  { value: "ark", label: tUI("consoleCopy.volcengineArk") },
  { value: "openrouter", label: "OpenRouter" },
] as const; }
export const PROVIDER_PROTOCOLS = providerProtocols();

function legacyAdapterLabels(tUI: CopyTranslator = defaultCopy): Record<string, string> { return {
  test: tUI("consoleCopy.sandboxEcho"),
  bifrost: "Bifrost",
}; }
const LEGACY_ADAPTER_LABELS = legacyAdapterLabels();

export function protocolOptions(current?: string, tUI: CopyTranslator = defaultCopy): { value: string; label: string }[] {
  const options = providerProtocols(tUI).map((item) => ({ value: item.value, label: item.label }));
  const value = (current || "").trim().toLowerCase();
  if (value && !options.some((item) => item.value === value)) {
    return [{ value, label: adapterLabel(value, tUI) }, ...options];
  }
  return options;
}

export function providerStatuses(tUI: CopyTranslator = defaultCopy) { return [
  { value: "active", label: tUI("consoleCopy.eligibleForRouting") },
  { value: "maintenance", label: tUI("consoleCopy.maintenanceExcludedFromRouting") },
  { value: "disabled", label: tUI("common.stDisabled") },
] as const; }
export const PROVIDER_STATUSES = providerStatuses();

/** 对齐 catalog.applyStrategy：priority / weight / price / health。 */
export function routeStrategies(tUI: CopyTranslator = defaultCopy) { return [
  { value: "priority", label: tUI("consoleCopy.specifiedOrder") },
  { value: "weight", label: tUI("consoleCopy.trafficWeights") },
  { value: "price", label: tUI("consoleCopy.lowestPrice") },
  { value: "health", label: tUI("consoleCopy.bestHealth") },
] as const; }
export const ROUTE_STRATEGIES = routeStrategies();

/** 网关 ResolveRoute 只取 status=active 的路由组。 */
export function routeStatuses(tUI: CopyTranslator = defaultCopy) { return [
  { value: "active", label: tUI("consoleCopy.routingEnabled") },
  { value: "inactive", label: tUI("staff.disable") },
] as const; }
export const ROUTE_STATUSES = routeStatuses();

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

export function routeStrategyOptions(current?: string, tUI: CopyTranslator = defaultCopy): { value: string; label: string }[] {
  return withCurrentOption(routeStrategies(tUI), current);
}

export function routeStatusOptions(current?: string, tUI: CopyTranslator = defaultCopy): { value: string; label: string }[] {
  return withCurrentOption(routeStatuses(tUI), current);
}

export function routeStrategyLabel(value?: string, tUI: CopyTranslator = defaultCopy): string {
  const key = (value || "").trim().toLowerCase();
  return routeStrategies(tUI).find((item) => item.value === key)?.label || value?.trim() || "—";
}

export function routeStatusLabel(value?: string, tUI: CopyTranslator = defaultCopy): string {
  const key = (value || "").trim().toLowerCase();
  return routeStatuses(tUI).find((item) => item.value === key)?.label || value?.trim() || "—";
}

export function modelStatusLabel(status?: string, tUI: CopyTranslator = defaultCopy): string {
  switch ((status || "").trim().toLowerCase()) {
    case "published":
      return tUI("common.stPublished");
    case "draft":
      return tUI("common.stDraft");
    case "deprecated":
      return tUI("common.stArchived");
    case "active":
      return tUI("keyUX.enable");
    case "inactive":
      return tUI("staff.disable");
    case "disabled":
      return tUI("common.stDisabled");
    default:
      return statusWord(status);
  }
}

export function providerKindLabel(kind?: string, tUI: CopyTranslator = defaultCopy): string {
  switch ((kind || "").trim().toLowerCase()) {
    case "aggregator":
      return tUI("consoleCopy.aggregator");
    case "direct":
      return tUI("consoleCopy.direct");
    default:
      return kind?.trim() || "—";
  }
}

export function adapterLabel(adapter?: string, tUI: CopyTranslator = defaultCopy): string {
  const value = (adapter || "").trim().toLowerCase();
  if (!value) {
    return "—";
  }
  const protocol = providerProtocols(tUI).find((item) => item.value === value);
  if (protocol) {
    return protocol.label;
  }
  return legacyAdapterLabels(tUI)[value] ?? (adapter?.trim() || "—");
}

export function providerStatusLabel(status?: string, tUI: CopyTranslator = defaultCopy): string {
  switch ((status || "").trim().toLowerCase()) {
    case "active":
      return tUI("consoleCopy.eligibleForRouting");
    case "maintenance":
      return tUI("consoleCopy.maintenance");
    case "disabled":
      return tUI("common.stDisabled");
    default:
      return status?.trim() || "—";
  }
}

export function healthLabel(health?: string, tUI: CopyTranslator = defaultCopy): string {
  switch ((health || "").trim().toLowerCase()) {
    case "available":
      return tUI("staff.active");
    case "degraded":
      return tUI("consoleCopy.degraded");
    case "unavailable":
      return tUI("consoleCopy.unavailable");
    case "maintenance":
      return tUI("consoleCopy.maintenance");
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

export function formatMappedModels(models?: MappedPublicModel[], tUI: CopyTranslator = defaultCopy): string {
  const ids = (models || []).map((item) => (item.public_id || "").trim()).filter(Boolean);
  return ids.length > 0 ? ids.join(" · ") : tUI("consoleCopy.noPublicModelsConnectedYet");
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

export function syncStateLabel(state?: string, tUI: CopyTranslator = defaultCopy): string {
  switch ((state || "").trim().toLowerCase()) {
    case "draft":
      return tUI("common.stDraft");
    case "reviewed":
      return tUI("consoleCopy.approved");
    case "rejected":
      return tUI("common.stRejected");
    case "published":
      return tUI("common.stPublished");
    default:
      return state?.trim() || "—";
  }
}

export function formatRpm(limit?: number, tUI: CopyTranslator = defaultCopy): string {
  if (!limit || limit <= 0) {
    return tUI("consoleCopy.unlimited");
  }
  return String(limit);
}

export function formatProviderSlugs(providers?: string[], tUI: CopyTranslator = defaultCopy): string {
  const items = (providers || []).map((item) => item.trim()).filter(Boolean);
  return items.length > 0 ? items.join(" · ") : tUI("consoleCopy.noProvidersConnectedYet");
}

export type ProviderOption = { id: string; name: string; slug: string };

export function filterProviderOptions(items: ProviderOption[], q: string, limit = 50): ProviderOption[] {
  const needle = q.trim().toLowerCase();
  const matched = needle
    ? items.filter((item) => [item.slug, item.name, item.id].some((part) => String(part || "").toLowerCase().includes(needle)))
    : items;
  return matched.slice(0, limit);
}
