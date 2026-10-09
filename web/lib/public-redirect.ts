import { safeNextPath } from "@/lib/login-next";

export type PublicRedirectSearch = Record<string, string | string[] | undefined>;

export function publicRedirectHref(destination: string, search: PublicRedirectSearch, pathModel?: string): string {
  const target = new URL(destination, "https://local.invalid");
  const value = (key: string) => {
    const raw = search[key];
    return (Array.isArray(raw) ? raw[0] : raw)?.trim() || "";
  };
  for (const key of ["promotion_code", "promo", "vendor", "kind", "q", "plan"]) {
    const raw = value(key);
    if (raw && !target.searchParams.has(key)) target.searchParams.set(key, raw);
  }
  const next = safeNextPath(value("next"));
  if (next) target.searchParams.set("next", next);
  const model = (pathModel ?? value("model")).trim();
  const parts = model.split("/");
  if (model && parts.every((part) => part && part !== "." && part !== "..")) {
    if (target.pathname === "/models") target.pathname += "/" + parts.map(encodeURIComponent).join("/");
    else target.searchParams.set("model", model);
  }
  return target.pathname + target.search;
}
