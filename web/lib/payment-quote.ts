import { formatUsdMinor } from "@/lib/money";

export type PaymentQuote = {
  adapter?: string;
  pay_major?: number;
  pay_currency?: string;
  pay_minor?: number;
  fee_minor?: number;
  wallet_minor?: number;
  credit_minor?: number;
};

export type QuotePhase = "idle" | "loading" | "ready" | "error";

export type QuoteSnapshot = {
  phase: QuotePhase;
  quote: PaymentQuote | null;
  message: string;
};

export function emptyQuoteSnapshot(): QuoteSnapshot {
  return { phase: "idle", quote: null, message: "" };
}

export function loadingQuoteSnapshot(): QuoteSnapshot {
  return { phase: "loading", quote: null, message: "" };
}

export function quoteMatchesSelection(
  quote: PaymentQuote | null | undefined,
  adapter: string,
  amount: number,
): boolean {
  if (!quote || !adapter || amount <= 0) {
    return false;
  }
  return quote.adapter === adapter && Number(quote.pay_major) === amount;
}

export function applyQuoteFetch(input: {
  generation: number;
  currentGeneration: number;
  adapter: string;
  amount: number;
  ok: boolean;
  quote?: PaymentQuote | null;
  message?: string;
}): QuoteSnapshot | null {
  if (input.generation !== input.currentGeneration) {
    return null;
  }
  if (!input.ok) {
    return { phase: "error", quote: null, message: input.message ?? "" };
  }
  const quote = input.quote ?? null;
  if (!quoteMatchesSelection(quote, input.adapter, input.amount)) {
    return { phase: "error", quote: null, message: input.message ?? "" };
  }
  return { phase: "ready", quote, message: "" };
}

export function formatPayMinor(currency: string | undefined, minor: number | undefined): string {
  const n = Number(minor) || 0;
  if (currency === "CNY") {
    return `¥${(n / 100).toFixed(2)}`;
  }
  return formatUsdMinor(n);
}

export function formatCreditMinor(minor: number | undefined): string {
  return formatUsdMinor(Number(minor) || 0);
}

export function formatAmountChip(currency: string | undefined, major: number): string {
  if (currency === "CNY") {
    return `¥${major}`;
  }
  return `$${major}`;
}
