import type { Metadata } from "next";
import { headers } from "next/headers";
import "./globals.css";
import { themeStyle, type Brand } from "@/lib/brand";
import { fetchAPI } from "@/lib/api";
import { Nav } from "@/app/nav";

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
  return (
    <html lang="zh-CN">
      <body className="min-h-screen antialiased" style={themeStyle(brand)}>
        <Nav brand={brand} />
        {children}
      </body>
    </html>
  );
}
