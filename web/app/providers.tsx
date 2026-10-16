"use client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NextIntlClientProvider } from "next-intl";
import { ThemeProvider } from "next-themes";
import { useState, type ReactNode } from "react";
import { ProductProvider } from "@/components/product/context";
export function AppProviders({ children }: { children: ReactNode }) {
  const [queryClient] = useState(() => new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 15_000 } } }));
  return <ThemeProvider attribute="class" defaultTheme="system" enableSystem><NextIntlClientProvider locale="zh-CN" messages={{}} timeZone="Asia/Shanghai"><QueryClientProvider client={queryClient}><ProductProvider>{children}</ProductProvider></QueryClientProvider></NextIntlClientProvider></ThemeProvider>;
}
