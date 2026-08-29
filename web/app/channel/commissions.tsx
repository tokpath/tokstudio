"use client";

import { useState } from "react";
import { apiBase } from "@/lib/api";

export default function ChannelCommissions() {
  const [message, setMessage] = useState("渠道管理员可看本渠道佣金和额度，看不到 prompt。");
  const [quota, setQuota] = useState<string>("—");

  async function refresh() {
    const [q, c] = await Promise.all([
      fetch(`${apiBase}/channel/quota`, { credentials: "include" }),
      fetch(`${apiBase}/channel/commissions`, { credentials: "include" }),
    ]);
    const qBody = await q.json();
    const cBody = await c.json();
    if (!q.ok && !c.ok) {
      setMessage(qBody.error?.message || "未登录渠道管理员");
      return;
    }
    setQuota(qBody.quota?.available_minor ?? "—");
    setMessage(`佣金流水 ${cBody.items?.length ?? 0} 条`);
  }

  return (
    <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
      <h2 className="mb-3 text-xl font-medium">渠道额度与佣金</h2>
      <p className="mb-3 text-sm text-slate-400">可用额度 {quota} micro-USD。佣金由平台承担，不扣渠道服务额度。</p>
      <button className="rounded border border-slate-600 px-4 py-2" onClick={refresh}>
        刷新
      </button>
      <p className="mt-3 text-sm text-slate-300">{message}</p>
    </section>
  );
}
