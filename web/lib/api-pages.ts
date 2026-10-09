import { apiClient } from "@/lib/client";

/** Complete management lookup lists; never silently return a truncated or failed page. */
export async function readAllPages<T>(path: string): Promise<{ items: T[] }> {
  const items: T[] = [];
  const seen = new Set<string>();
  let cursor = "";
  do {
    const separator = path.includes("?") ? "&" : "?";
    const page = await apiClient<{ items?: T[]; next_cursor?: string; error?: { message?: string } }>("GET", `${path}${separator}limit=100${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`);
    if (page.error || !Array.isArray(page.items)) throw new Error(page.error?.message || "Could not load list");
    items.push(...page.items);
    cursor = page.next_cursor || "";
    if (cursor && seen.has(cursor)) throw new Error("List pagination did not advance");
    if (cursor) seen.add(cursor);
  } while (cursor);
  return { items };
}
