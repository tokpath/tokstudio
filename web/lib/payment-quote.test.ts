import { describe, expect, it } from "vitest";
import {
  applyQuoteFetch,
  formatAmountChip,
  formatPayMinor,
  loadingQuoteSnapshot,
  quoteMatchesSelection,
} from "./payment-quote";

const alipay100 = {
  adapter: "alipay",
  pay_major: 100,
  pay_currency: "CNY",
  pay_minor: 10000,
  fee_minor: 0,
  credit_minor: 13_986_013,
};

describe("quote matching", () => {
  it("rejects a quote for a different amount or adapter", () => {
    expect(quoteMatchesSelection(alipay100, "alipay", 100)).toBe(true);
    expect(quoteMatchesSelection(alipay100, "alipay", 300)).toBe(false);
    expect(quoteMatchesSelection(alipay100, "stripe", 100)).toBe(false);
    expect(quoteMatchesSelection(null, "alipay", 100)).toBe(false);
  });

  it("ignores a stale response after the selection has moved on", () => {
    const stale = applyQuoteFetch({
      generation: 1,
      currentGeneration: 2,
      adapter: "alipay",
      amount: 100,
      ok: true,
      quote: alipay100,
    });
    expect(stale).toBeNull();
  });

  it("does not keep a mismatched quote even if the request is current", () => {
    const next = applyQuoteFetch({
      generation: 2,
      currentGeneration: 2,
      adapter: "alipay",
      amount: 300,
      ok: true,
      quote: alipay100,
    });
    expect(next?.phase).toBe("error");
    expect(next?.quote).toBeNull();
  });

  it("accepts only the quote that matches the current selection", () => {
    const next = applyQuoteFetch({
      generation: 2,
      currentGeneration: 2,
      adapter: "alipay",
      amount: 100,
      ok: true,
      quote: alipay100,
    });
    expect(next).toEqual({ phase: "ready", quote: alipay100, message: "" });
  });

  it("starts a new selection with no leftover quote", () => {
    expect(loadingQuoteSnapshot()).toEqual({ phase: "loading", quote: null, message: "" });
  });
});

describe("money labels", () => {
  it("formats chips and pay minors with currency", () => {
    expect(formatAmountChip("CNY", 100)).toBe("¥100");
    expect(formatAmountChip("USD", 10)).toBe("$10");
    expect(formatPayMinor("CNY", 10000)).toBe("¥100.00");
    expect(formatPayMinor("USD", 10_000_000)).toBe("$10.00");
    expect(formatPayMinor("USD", 26)).toBe("$0.000026");
  });
});
