"use client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { I18nProvider } from "@/components/i18n";
import type { Locale, LocaleDictionary } from "@/lib/i18n";
import { ThemeProvider } from "next-themes";
import { useState, type ReactNode } from "react";
import { ProductProvider } from "@/components/product/context";
export function AppProviders({ children, locale, dictionary, chineseDictionary }: { children: ReactNode; locale: Locale; dictionary: LocaleDictionary; chineseDictionary: LocaleDictionary }) {
  // Returning to a tab must not refetch every mounted query at once. That burst
  // blocks the click that woke the page. Live screens keep their own intervals.
  const [queryClient] = useState(() => new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 60_000, refetchOnWindowFocus: false } } }));
  return <ThemeProvider attribute="class" defaultTheme="system" enableSystem><I18nProvider initialLocale={locale} dictionary={dictionary} chineseDictionary={chineseDictionary}><QueryClientProvider client={queryClient}><ProductProvider>{children}</ProductProvider></QueryClientProvider></I18nProvider></ThemeProvider>;
}
