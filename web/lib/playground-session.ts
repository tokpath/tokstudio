import type { CatalogModel } from "@/lib/catalog";

/** Keep the catalog pick even when it is not in the current option list. Never invent a model. */
export function pickPlaygroundModel(models: CatalogModel[], requested?: string): string {
  const asked = decodeModelId(requested);
  if (asked) {
    return asked;
  }
  return models[0]?.id || "";
}

export function decodeModelId(requested?: string): string {
  if (!requested) {
    return "";
  }
  try {
    return decodeURIComponent(requested).trim();
  } catch {
    return requested.trim();
  }
}
