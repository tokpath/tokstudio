import { defaultCopy, type CopyTranslator } from "@/lib/copy";
export const COST_SOURCE = "TokenHub";
export function emptyAttemptCostText(tUI: CopyTranslator = defaultCopy) { return tUI("adminUi.marginEmpty"); }
export const EMPTY_ATTEMPT_COST = emptyAttemptCostText();
export function marginPageTitle(tUI: CopyTranslator = defaultCopy) { return tUI("admin.margin"); }
export const PAGE_TITLE = marginPageTitle();

export type PriceTiers = {
  upstream?: string;
  wholesale?: string;
  sell?: string;
  channel?: string;
};

export type AttemptCostRow = {
  attempt_id?: string;
  request_id?: string;
  cost_source?: string;
  cost_minor?: number;
  sell_minor?: number;
  margin_minor?: number;
  missing_cost?: boolean;
  prices?: PriceTiers;
  public_model_id?: string;
  provider_id?: string;
  upstream_model_id?: string;
  fact_source?: string;
  state?: string;
};

export type MarginView = {
  attempt_cost_minor?: number;
  sell_minor?: number;
  margin_minor?: number;
  pending_count?: number;
  items?: AttemptCostRow[];
};

const FORBIDDEN = ["estimate_debit", "blind_debit", "估扣", "估算扣款", "estimate-debit", "手填成本"];

export function marginMinor(sell: number, cost: number): number {
  return sell - cost;
}

export function isNegativeMargin(margin: number): boolean {
  return margin < 0;
}

export function marginClassName(margin: number): string {
  return isNegativeMargin(margin) ? "text-[color:var(--danger)]" : "";
}

export function emptyAttemptCostTitle(tUI: CopyTranslator = defaultCopy): string {
  return emptyAttemptCostText(tUI);
}

export function pageTitle(tUI: CopyTranslator = defaultCopy): string {
  return marginPageTitle(tUI);
}

export function costSourceLabel(): string {
  return COST_SOURCE;
}

export function forbidsEstimateCost(labels: string[]): boolean {
  return !labels.some((label) => FORBIDDEN.some((banned) => label.includes(banned)));
}

export function isAdminOnlyMarginPath(pathname: string): boolean {
  return pathname === "/admin/margin" || pathname.startsWith("/admin/margin/");
}

export function correctionKinds(): Array<"fill_cost" | "adjust_margin"> {
  return ["fill_cost", "adjust_margin"];
}
