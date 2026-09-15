/** 总览：有真实用量才切回快捷入口；余额未知不当成「不足」。 */
export function overviewHasUsage(events: unknown[] | null | undefined): boolean {
  return Array.isArray(events) && events.length > 0;
}

export function overviewNeedsTopup(available?: string | null): boolean {
  if (available == null || available === "") {
    return false;
  }
  const amount = Number(available);
  return Number.isFinite(amount) && amount <= 0;
}

export function keysCreateQueryOpen(search: string): boolean {
  const query = search.startsWith("?") ? search.slice(1) : search;
  return new URLSearchParams(query).get("create") === "1";
}
