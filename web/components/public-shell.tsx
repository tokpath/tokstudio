import type { ReactNode } from "react";
import { PublicHeader } from "@/components/public-header";

export function PublicShell({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-screen flex-col">
      <PublicHeader />
      <div className="mx-auto flex w-full max-w-[1120px] flex-1 flex-col px-6 py-16">{children}</div>
      <footer className="border-t border-hairline">
        <div className="mx-auto flex max-w-[1120px] flex-col gap-3 px-6 py-8 text-[13px] text-ink-mute sm:flex-row sm:items-center sm:justify-between">
          <p>TokenHub Clearing · 纸/碳双主题 · 四个入口共用同一套语法</p>
          <p className="flex flex-wrap gap-3">
            <a href="/console/user">用户台</a>
            <a href="/console/channel">渠道台</a>
            <a href="/console/admin">管理台</a>
          </p>
        </div>
      </footer>
    </div>
  );
}
