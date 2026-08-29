"use client";

import { AdminShell } from "../shell";
import { apiClient } from "@/lib/client";

export default function AdminSettingsPage() {
  async function setup2FA() {
    await apiClient("POST", "/admin/me/2fa/setup", { method: "POST" });
  }
  function setLocale(locale: string) {
    document.cookie = `NEXT_LOCALE=${locale}; path=/; max-age=31536000`;
    window.location.reload();
  }
  return (
    <AdminShell>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-3 text-xl font-medium">系统设置</h2>
        <p className="mb-4 text-sm text-slate-400">管理员 2FA 使用 TOTP。语言预留中 / 英 / 日（next-intl）。</p>
        <div className="flex flex-wrap gap-2">
          <button className="rounded border border-slate-600 px-3 py-2" onClick={setup2FA}>
            启用 2FA
          </button>
          <button className="rounded border border-slate-600 px-3 py-2" onClick={() => setLocale("zh")}>
            中文
          </button>
          <button className="rounded border border-slate-600 px-3 py-2" onClick={() => setLocale("en")}>
            English
          </button>
          <button className="rounded border border-slate-600 px-3 py-2" onClick={() => setLocale("ja")}>
            日本語
          </button>
        </div>
      </section>
    </AdminShell>
  );
}
