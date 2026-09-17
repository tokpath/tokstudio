import type { CatalogModel } from "@/lib/catalog";
import { catalogModelUsable, modelEntry } from "@/lib/model-use";

export type PlaygroundPick = {
  id: string;
  error?: "missing" | "unavailable" | "notChat";
};

/** URL 模型必须出现在已加载目录里。不把未知 id 补成可选项。 */
export function pickPlaygroundModel(models: CatalogModel[], requested?: string): PlaygroundPick {
  const asked = decodeModelId(requested);
  if (asked) {
    const found = models.find((item) => item.id === asked);
    if (!found) {
      return { id: "", error: "missing" };
    }
    if (!catalogModelUsable(found)) {
      return { id: found.id, error: "unavailable" };
    }
    if (modelEntry(found) !== "chat") {
      return { id: found.id, error: "notChat" };
    }
    return { id: found.id };
  }
  const chat = models.find((item) => catalogModelUsable(item) && modelEntry(item) === "chat");
  return { id: chat?.id || "" };
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
