import type { Metadata } from "next";
import "./globals.css";
import { AppProviders } from "./providers";
export const metadata: Metadata = { title: "TokenHub", description: "品牌模型服务、独立经营与统一资产账务" };
export default function RootLayout({ children }: { children: React.ReactNode }) { return <html lang="zh-CN" suppressHydrationWarning><body className="min-h-screen bg-canvas font-sans text-ink antialiased"><AppProviders>{children}</AppProviders></body></html>; }
