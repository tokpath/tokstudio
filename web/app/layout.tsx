import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "TokenHub",
  description: "TokenHub 控制面状态",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body className="min-h-screen antialiased">{children}</body>
    </html>
  );
}
