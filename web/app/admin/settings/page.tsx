"use client";

import { useState } from "react";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";

export default function AdminSettingsPage() {
  const [rate, setRate] = useState("0.5");
  const [minReq, setMinReq] = useState("5");
  const [pending, setPending] = useState("1");
  const [message, setMessage] = useState("告警阈值写入 ops 表，评估成功率时会读取。");

  async function setup2FA() {
    await apiClient("POST", "/admin/me/2fa/setup", { method: "POST" });
  }
  function setLocale(locale: string) {
    document.cookie = `NEXT_LOCALE=${locale}; path=/; max-age=31536000`;
    window.location.reload();
  }
  async function loadThresholds() {
    const res = await fetch(`${apiBase}/admin/ops/thresholds`, { credentials: "include" });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "未登录平台管理员");
      return;
    }
    setRate(String(body.thresholds?.success_rate_min ?? 0.5));
    setMinReq(String(body.thresholds?.min_requests ?? 5));
    setPending(String(body.thresholds?.pending_count ?? 1));
    setMessage("已读取告警阈值");
  }
  async function saveThresholds() {
    const res = await fetch(`${apiBase}/admin/ops/thresholds`, {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: JSON.stringify({
        success_rate_min: Number(rate),
        min_requests: Number(minReq),
        pending_count: Number(pending),
      }),
    });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "保存失败");
      return;
    }
    setMessage(`已保存：成功率 < ${body.thresholds?.success_rate_min} 且请求 ≥ ${body.thresholds?.min_requests}`);
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
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-3 text-xl font-medium">告警阈值</h2>
        <p className="mb-3 text-sm text-slate-400">成功率下限、最少请求数、待对账条数。保存需要二次确认头。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <input className="w-28 rounded bg-slate-900 p-2" value={rate} onChange={(e) => setRate(e.target.value)} aria-label="成功率下限" />
          <input className="w-28 rounded bg-slate-900 p-2" value={minReq} onChange={(e) => setMinReq(e.target.value)} aria-label="最少请求数" />
          <input className="w-28 rounded bg-slate-900 p-2" value={pending} onChange={(e) => setPending(e.target.value)} aria-label="待对账条数" />
          <button className="rounded border border-slate-600 px-3 py-2" onClick={loadThresholds}>
            读取阈值
          </button>
          <button className="rounded border border-slate-600 px-3 py-2" onClick={saveThresholds}>
            保存阈值
          </button>
        </div>
        <p className="text-sm text-slate-300">{message}</p>
      </section>
    </AdminShell>
  );
}
