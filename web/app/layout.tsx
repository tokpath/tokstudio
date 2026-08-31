import type { Metadata } from "next";
import { cookies, headers } from "next/headers";
import { Geist_Mono, Inter } from "next/font/google";
import "./globals.css";
import { themeStyle, type Brand } from "@/lib/brand";
import { fetchAPI } from "@/lib/api";
import { messagesFor, resolveLocale } from "@/lib/i18n";
import { AppChrome } from "@/components/layout/app-chrome";
import { AppProviders } from "@/app/providers";

const sans = Inter({ subsets: ["latin"], variable: "--font-sans" });
const mono = Geist_Mono({ subsets: ["latin"], variable: "--font-mono" });

export const metadata: Metadata = {
  title: "TokenHub",
  description: "一个 Key，可解释路由，账能复算。",
};

async function loadBrand(): Promise<Brand | undefined> {
  try {
    const host = (await headers()).get("x-tokenhub-host") || (await headers()).get("host") || "localhost";
    const data = await fetchAPI<{ brand: Brand }>("/v1/public/brand", { host });
    return data.brand;
  } catch {
    return undefined;
  }
}

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const brand = await loadBrand();
  const locale = resolveLocale((await cookies()).get("NEXT_LOCALE")?.value);
  return (
    <html lang={locale === "zh" ? "zh-CN" : locale} suppressHydrationWarning>
      <body className={`${sans.variable} ${mono.variable} min-h-screen bg-canvas font-sans text-ink antialiased`} style={themeStyle(brand)}>
        <AppProviders locale={locale} messages={messagesFor(locale)}>
          <AppChrome brand={brand}>{children}</AppChrome>
        </AppProviders>
      </body>
    </html>
  );
}
