import type { ResourcePage } from "@/lib/contracts";

// Only an unfinished publication needs polling. Stop once it applies or needs
// operator attention, so ordinary pricing pages do not keep making requests.
export function pricingRefetchInterval(page?: ResourcePage): number | false {
  return page?.items.some(row => Array.isArray(row.values.pending_changes) && row.values.pending_changes.some(change =>
    change && typeof change === "object" && !Array.isArray(change) &&
    ["announcing", "announced"].includes(String(change.status)) && change.needs_attention !== true
  )) ? 3000 : false;
}
