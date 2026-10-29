"use client";

import { createContext, useContext, type ReactNode } from "react";
import { defaultMonetary, type Monetary } from "@/lib/contracts";

const MonetaryContext = createContext<Monetary>(defaultMonetary);
export function MonetaryProvider({ value, children }: { value?: Monetary; children: ReactNode }) {
  return <MonetaryContext.Provider value={value || defaultMonetary}>{children}</MonetaryContext.Provider>;
}
export function useMonetary() { return useContext(MonetaryContext); }

export function monetaryRefetchInterval(monetary?: Monetary): number | false {
  if (monetary?.status !== "scheduled") return false;
  const remaining = monetary.effective_at ? new Date(monetary.effective_at).getTime() - Date.now() : NaN;
  return Number.isFinite(remaining) ? Math.min(10000, Math.max(1000, remaining + 100)) : 10000;
}
