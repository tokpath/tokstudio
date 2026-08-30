"use client";

import { useState } from "react";
import { apiBase } from "@/lib/api";

type Allocation = {
  id?: string;
  user_id?: string;
  granted_minor?: number;
  consumed_minor?: number;
  remaining_minor?: number;
  status?: string;
};

export default function ChannelCommissions() {
  const [message, setMessage] = useState("渠道管理员可看本渠道佣金和额度，看不到 prompt。");
  const [quota, setQuota] = useState<string>("—");
  const [issued, setIssued] = useState<string>("—");
  const [consumed, setConsumed] = useState<string>("—");
  const [allocations, setAllocations] = useState<Allocation[]>([]);

  async function refresh() {
    const [q, c, a] = await Promise.all([
      fetch(`${apiBase}/channel/quota`, { credentials: "include" }),
      fetch(`${apiBase}/channel/commissions`, { credentials: "include" }),
      fetch(`${apiBase}/channel/allocations`, { credentials: "include" }),
    ]);
    const qBody = await q.json();
    const cBody = await c.json();
    const aBody = await a.json();
    if (!q.ok && !c.ok) {
      setMessage(qBody.error?.message || "未登录渠道管理员");
      return;
    }
    setQuota(qBody.quota?.available_minor ?? "—");
    setIssued(qBody.quota?.issued_minor ?? "—");
    setConsumed(qBody.quota?.consumed_minor ?? "—");
    setAllocations(Array.isArray(aBody.items) ? aBody.items : []);
    setMessage(`佣金流水 ${cBody.items?.length ?? 0} 条，已发放 ${aBody.items?.length ?? 0} 笔`);
  }

  return (
    <section className="rounded-2xl border border-white/10 bg-white/[0.035] shadow-glow p-6">
      <h2 className="mb-3 text-xl font-medium">渠道额度与佣金</h2>
      <p className="mb-3 text-sm text-slate-400">
        可用额度 {quota} micro-USD。用户充值时按 1:1 发放服务额度，聊天不再二次扣渠道。佣金由平台承担。
      </p>
      <p className="mb-3 text-sm text-slate-400">
        已发放 {issued}，已消费 {consumed}。
      </p>
      <h3 className="mb-2 text-lg font-medium">已发放额度</h3>
      <ul className="mb-3 space-y-1 text-sm text-slate-300">
        {allocations.length === 0 ? (
          <li>还没有发放记录。点刷新后可看到下属用户充值对应的额度。</li>
        ) : (
          allocations.map((item) => (
            <li key={item.id}>
              {item.user_id}：发放 {item.granted_minor} / 已用 {item.consumed_minor} / 剩余 {item.remaining_minor}（{item.status}）
            </li>
          ))
        )}
      </ul>
      <button className="rounded border border-slate-600 px-4 py-2" onClick={refresh}>
        刷新
      </button>
      <p className="mt-3 text-sm text-slate-300">{message}</p>
    </section>
  );
}
