import { defaultCopy, type CopyTranslator } from "@/lib/copy";
/** 管理台目录三页共用的业务用语。同一对象只保留一个中文名。 */
export function catalogLabels(tUI: CopyTranslator = defaultCopy) { return {
  publicModelId: tUI("consoleCopy.publicModelId"),
  displayName: tUI("shell.displayName"),
  vendor: tUI("modelService.vendor"),
  provider: tUI("admin.providers"),
  providerPool: tUI("consoleCopy.providerPool"),
  upstreamModelId: tUI("consoleCopy.upstreamModelId"),
  routeStrategy: tUI("consoleCopy.routingStrategy"),
  routeStatus: tUI("consoleCopy.routeStatus"),
  tokenizer: tUI("consoleCopy.tokenizer"),
} as const; }
export const CATALOG_LABEL = catalogLabels();

export function catalogHelp(tUI: CopyTranslator = defaultCopy) { return {
  providers: tUI("consoleCopy.providersAreUpstreamSourcesConfigureProtocolsUrlsAnd"),
  models: tUI("consoleCopy.configureThePublicModelSNameTypeCapabilities"),
  routes: tUI("consoleCopy.configureProvidersUpstreamModelIdsAndRoutingStrategies"),
} as const; }
export const CATALOG_HELP = catalogHelp();

export type PublicModelOption = { id: string; display_name?: string; vendor?: string };

export function slugifyCatalogId(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[\s_]+/g, "-")
    .replace(/[^a-z0-9.-]+/g, "")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
}

export function suggestPublicId(vendor: string, current: string, previousVendor = ""): string {
  const nextVendor = vendor.trim().toLowerCase();
  const value = current.trim();
  if (!nextVendor) {
    return value;
  }
  const nextPrefix = `${nextVendor}/`;
  const previous = previousVendor.trim().toLowerCase();
  if (!value) {
    return nextPrefix;
  }
  if (previous) {
    const oldPrefix = `${previous}/`;
    if (value === previous || value === oldPrefix) {
      return nextPrefix;
    }
    if (value.startsWith(oldPrefix)) {
      return `${nextPrefix}${value.slice(oldPrefix.length)}`;
    }
  }
  if (!value.includes("/")) {
    return `${nextPrefix}${value}`;
  }
  return value;
}

export function suggestPublicIdFromDisplay(
  vendor: string,
  displayName: string,
  current: string,
  previousSlug = "",
): string {
  const nextVendor = vendor.trim().toLowerCase();
  const slug = slugifyCatalogId(displayName);
  if (!nextVendor) {
    return current.trim();
  }
  const auto = slug ? `${nextVendor}/${slug}` : `${nextVendor}/`;
  const value = current.trim();
  const previousAuto = previousSlug ? `${nextVendor}/${previousSlug}` : `${nextVendor}/`;
  if (!value || value === `${nextVendor}/` || value === previousAuto) {
    return auto;
  }
  return value;
}

export function filterPublicModelOptions(items: PublicModelOption[], q: string, limit = 50): PublicModelOption[] {
  const needle = q.trim().toLowerCase();
  const matched = needle
    ? items.filter((item) =>
        [item.id, item.display_name, item.vendor].some((part) => String(part || "").toLowerCase().includes(needle)),
      )
    : items;
  return matched.slice(0, limit);
}
