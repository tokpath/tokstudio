import { apiClient } from "@/lib/client";

export type PlanChannel = { id: string; code: string; type?: string; status: string; parent_id?: string };

export async function listPlanChannels(): Promise<PlanChannel[]> {
  const items: PlanChannel[] = [];
  const seen = new Set<string>();
  let cursor = "";
  do {
    const path = `/admin/channels?limit=100${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`;
    const page = await apiClient<{ items?: PlanChannel[]; next_cursor?: string }>("GET", path);
    items.push(...(page.items ?? []));
    cursor = page.next_cursor ?? "";
    if (!cursor || seen.has(cursor)) break;
    seen.add(cursor);
  } while (true);
  return items.filter((channel) => channel.status === "active");
}

export async function listEligiblePlanChannels(): Promise<PlanChannel[]> {
  const response = await apiClient<{ items?: PlanChannel[] }>("GET", "/admin/plans/eligible-channels");
  return response.items ?? [];
}
