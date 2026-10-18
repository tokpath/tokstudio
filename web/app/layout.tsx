import type { Metadata } from "next";
import { cookies, headers } from "next/headers";
import "./globals.css";
import { AppProviders } from "./providers";
import { localeCookie, localeTags, resolveLocale, translate } from "@/lib/i18n";
async function requestLocale() { const [cookieStore, requestHeaders] = await Promise.all([cookies(), headers()]); return resolveLocale(cookieStore.get(localeCookie)?.value, requestHeaders.get("accept-language") || ""); }
export async function generateMetadata(): Promise<Metadata> { const locale = await requestLocale(); return { title: "TokenHub", description: translate(locale, "品牌模型服务、独立经营与统一资产账务") }; }
export default async function RootLayout({ children }: { children: React.ReactNode }) { const locale = await requestLocale(); return <html lang={localeTags[locale]} suppressHydrationWarning><body className="min-h-screen bg-canvas font-sans text-ink antialiased"><AppProviders locale={locale}>{children}</AppProviders></body></html>; }
