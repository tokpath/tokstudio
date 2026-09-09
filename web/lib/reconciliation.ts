export type UsageGap = {
  id?: string;
  request_id?: string;
  api_key_id?: string;
  public_model_id?: string;
  channel_org_id?: string;
  state?: string;
  occurred_at?: string;
  customer_amount_minor?: number;
  reserved_minor?: number;
  settled_minor?: number;
  auth_status?: string;
  missing_usage?: boolean;
  unit_usage?: unknown;
  unit_prices?: unknown;
};

export type PendingFilters = {
  status?: string;
  from?: string;
  to?: string;
};

export function pendingListPath(filters: PendingFilters): string {
  const qs = new URLSearchParams();
  if (filters.status && filters.status !== "pending_reconciliation") {
    qs.set("status", filters.status);
  }
  if (filters.from?.trim()) qs.set("from", filters.from.trim());
  if (filters.to?.trim()) qs.set("to", filters.to.trim());
  const suffix = qs.toString();
  return suffix ? `/admin/usage/pending?${suffix}` : "/admin/usage/pending";
}

export function statementListPath(filters: {
  apiKeyId?: string;
  modelId?: string;
  channelId?: string;
  userId?: string;
  state?: string;
  from?: string;
  to?: string;
}): string {
  const qs = new URLSearchParams();
  if (filters.apiKeyId?.trim()) qs.set("api_key_id", filters.apiKeyId.trim());
  if (filters.modelId?.trim()) qs.set("public_model_id", filters.modelId.trim());
  if (filters.channelId?.trim()) qs.set("channel_id", filters.channelId.trim());
  if (filters.userId?.trim()) qs.set("user_id", filters.userId.trim());
  if (filters.state?.trim()) qs.set("state", filters.state.trim());
  if (filters.from?.trim()) qs.set("from", filters.from.trim());
  if (filters.to?.trim()) qs.set("to", filters.to.trim());
  const suffix = qs.toString();
  return suffix ? `/admin/usage?${suffix}` : "/admin/usage";
}

export function gapKey(row: UsageGap): string {
  return row.id || row.request_id || "";
}
