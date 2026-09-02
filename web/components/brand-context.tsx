"use client";

import { createContext, useContext } from "react";
import type { Brand } from "@/lib/brand";

const BrandContext = createContext<Brand | undefined>(undefined);

export function BrandProvider({ brand, children }: { brand?: Brand; children: React.ReactNode }) {
  return <BrandContext.Provider value={brand}>{children}</BrandContext.Provider>;
}

export function useBrand() {
  return useContext(BrandContext);
}
