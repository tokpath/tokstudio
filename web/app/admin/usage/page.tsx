"use client";

import { useState } from "react";
import { ConfirmButton } from "@/components/confirm-button";
import { Input } from "@/components/ui/input";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";

type Usage = { id: string; request_id: string; state: string; customer_amount?: number };

export default function AdminUsagePage() {
  const [requestID, setRequestID] = useState("");
  const [prompt, setPrompt] = useState("8");
  const [completion, setCompletion] = useState("4");
  const [message, setMessage] = useState("待对账必须按真实 usage 回放，禁止按估算扣款。");

  async function replay() {
    const res = await fetch(`${apiBase}/admin/usage/replay`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({
        request_id: requestID,
        usage: { prompt_tokens: Number(prompt), completion_tokens: Number(completion) },
      }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已回放 ${body.item?.usage_event_id || requestID} → ${body.item?.state}` : body.error?.message || "回放失败");
  }

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <h2 className="mb-3 text-xl font-medium">用量回放</h2>
        <p className="mb-3 text-sm text-ink-secondary">对 pending_reconciliation 按 request_id 补真实 Token。重复回放幂等，不会双扣。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-72" value={requestID} onChange={(e) => setRequestID(e.target.value)} aria-label="request_id" placeholder="request_id" />
          <Input className="w-24" value={prompt} onChange={(e) => setPrompt(e.target.value)} aria-label="prompt tokens" />
          <Input className="w-24" value={completion} onChange={(e) => setCompletion(e.target.value)} aria-label="completion tokens" />
          <ConfirmButton size="sm" title="确认回放 usage" description="只对 pending_reconciliation 按真实 Token 回放，不会双扣。" onConfirm={replay}>
            回放 usage
          </ConfirmButton>
        </div>
        <p className="text-sm text-ink-secondary">{message}</p>
      </section>
      <AdminListPanel<Usage>
        path="/admin/usage"
        title="用量 / 账单"
        columns={[
          { accessorKey: "request_id", header: "Request" },
          { accessorKey: "state", header: "State" },
          { accessorKey: "id", header: "ID" },
        ]}
      />
    </AdminShell>
  );
}
