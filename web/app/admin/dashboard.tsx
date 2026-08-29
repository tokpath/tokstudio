"use client";

import { useState } from "react";
import { apiBase } from "@/lib/api";
import { formatDashboard } from "@/lib/dashboard";

export default function AdminDashboard() {
  const [message, setMessage] = useState("按 Provider / 模型 / 渠道 / 用户 / API Key 看成功率、延迟、收入和毛利。");

  async function refresh() {
    const res = await fetch(`${apiBase}/admin/ops/dashboard`, { credentials: "include" });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "未登录平台管理员");
      return;
    }
    setMessage(formatDashboard(body.dashboard));
  }

  return (
    <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
      <h2 className="mb-3 text-xl font-medium">运营看板</h2>
      <p className="mb-3 text-sm text-slate-400">告警和 runbook 在 /admin/ops/alerts 与 /admin/ops/runbooks。</p>
      <button className="rounded border border-slate-600 px-4 py-2" onClick={refresh}>
        刷新指标
      </button>
      <p className="mt-3 text-sm text-slate-300">{message}</p>
    </section>
  );
}
