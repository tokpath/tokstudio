/** 管理台目录三页共用的业务用语。同一对象只保留一个中文名。 */
export const CATALOG_LABEL = {
  publicModelId: "公开模型标识",
  displayName: "显示名",
  vendor: "原厂",
  provider: "提供商",
  providerSlug: "提供商标识",
  providerPool: "提供商池",
  upstreamModelId: "上游模型标识",
  routeStrategy: "选路策略",
  routeStatus: "路由状态",
  tokenizer: "分词器",
} as const;

export const CATALOG_HELP = {
  providers: "提供商是进货渠道：协议、调用地址和密钥都在这里。客户看到的名字在模型页；选哪几家、谁优先在路由组。",
  models: "客户看到的是公开模型。真正打到哪家上游，用提供商和上游模型标识决定。选路顺序在路由组。",
  routes: "为已有公开模型排提供商池。上游模型标识在模型页配置，这里只决定走哪几家、谁优先。",
} as const;

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

export function suggestProviderSlug(name: string, current: string, previousSlug = ""): string {
  const next = slugifyCatalogId(name);
  const value = current.trim();
  if (!value || value === previousSlug) {
    return next;
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
