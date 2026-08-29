import type { Metadata } from "next";
import { cookies, headers } from "next/headers";
import "./globals.css";
import { themeStyle, type Brand } from "@/lib/brand";
import { fetchAPI } from "@/lib/api";
import { messagesFor, resolveLocale } from "@/lib/i18n";
import { Nav } from "@/app/nav";
import { AppProviders } from "@/app/providers";

export const metadata: Metadata = {
  title: "TokenHub",
  description: "TokenHub 多门户",
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
    <html lang={locale === "zh" ? "zh-CN" : locale}>
      <body className="min-h-screen antialiased" style={themeStyle(brand)}>
        <AppProviders locale={locale} messages={messagesFor(locale)}>
          <Nav brand={brand} />
          {children}
        </AppProviders>
      </body>
    </html>
  );
}
