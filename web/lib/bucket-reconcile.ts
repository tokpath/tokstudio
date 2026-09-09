export type ThreeBuckets = {
  available_minor?: number;
  reserved_minor?: number;
  withdrawable_minor?: number;
};

export type WindowUsageTotals = {
  requests?: number;
  customer_minor?: number;
  charge_minor?: number;
  reserved_minor?: number;
  pending_count?: number;
};

export type DiffRow = {
  request_id?: string;
  usage_id?: string;
  occurred_at?: string;
  public_model_id?: string;
  api_key_id?: string;
  state?: string;
  usage_minor?: number;
  charge_minor?: number;
  ledger_debit_minor?: number;
  reserved_minor?: number;
  match?: boolean;
  status?: "match" | "mismatch" | string;
  already_pending?: boolean;
  missing_usage?: boolean;
};

export type ReconcileView = {
  buckets?: ThreeBuckets;
  usage_totals?: WindowUsageTotals;
  items?: DiffRow[];
  pending?: DiffRow[];
};

export type ReconcileAction = "flag_pending";

const FORBIDDEN_ACTIONS = ["estimate_debit", "blind_debit", "估扣", "估算扣款"];

export function isMatchRow(row: DiffRow): boolean {
  return row.match === true || row.status === "match";
}

export function rowTone(row: DiffRow): "success" | "danger" {
  return isMatchRow(row) ? "success" : "danger";
}

export function rowClassName(row: DiffRow): string {
  return isMatchRow(row) ? "bg-success/10 text-success" : "bg-danger/10 text-danger";
}

/** 只有差异能送进 pending_reconciliation。匹配行无动作。永不估扣。 */
export function reconcileActions(row: DiffRow): ReconcileAction[] {
  if (isMatchRow(row)) {
    return [];
  }
  return ["flag_pending"];
}

export function forbidsEstimateDebit(labels: string[]): boolean {
  return !labels.some((label) => FORBIDDEN_ACTIONS.some((banned) => label.includes(banned)));
}

export function emptyUsageTitle(): string {
  return "暂无 usage";
}

export function pageTitle(): string {
  return "对账";
}

export function pendingQueueTitle(): string {
  return "待对账队列";
}

export function flagPath(scope: "user" | "channel"): string {
  return scope === "channel" ? "/channel/reconciliation/flag" : "/v1/me/reconciliation/flag";
}

export function listPath(scope: "user" | "channel"): string {
  return scope === "channel" ? "/channel/reconciliation" : "/v1/me/reconciliation";
}

export function diffKey(row: DiffRow): string {
  return row.usage_id || row.request_id || "";
}

export function tabularMinor(value: number | undefined): string {
  return String(value ?? 0);
}
