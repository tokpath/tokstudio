"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";

type Channel = { id: string; code: string; type: string; status: string; brand_id: string };

export default function AdminChannelsPage() {
  const queryClient = useQueryClient();
  const [channelID, setChannelID] = useState("chn_reseller_b");
  const [amount, setAmount] = useState("1000000");
  const [message, setMessage] = useState("渠道额度按 micro-USD。发放和扣减都要二次确认。");
  const [channelMessage, setChannelMessage] = useState("创建和改状态都要二次确认。不要停用 chn_official_a / chn_reseller_b / chn_oem_c。");

  async function loadQuota() {
    const res = await fetch(`${apiBase}/admin/channel-quotas/${channelID}`, { credentials: "include" });
    const body = await res.json();
    setMessage(res.ok ? `可用额度 ${body.quota?.available_minor}` : body.error?.message || "读取额度失败");
  }

  async function grant() {
    const res = await fetch(`${apiBase}/admin/channel-quotas/grant`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: JSON.stringify({ channel_org_id: channelID, amount_minor: Number(amount) }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已调整 ${channelID}，可用 ${body.quota?.available_minor}` : body.error?.message || "调整失败");
  }

  return (
    <AdminShell>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-3 text-xl font-medium">渠道额度</h2>
        <p className="mb-3 text-sm text-slate-400">B/C 渠道可用额度在用户充值时按 1:1 发放。正数授予，负数扣减。额度不足时不能再给新用户发放，预授权也会失败。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-56" value={channelID} onChange={(e) => setChannelID(e.target.value)} aria-label="渠道 ID" placeholder="chn_..." />
          <Input className="w-40" value={amount} onChange={(e) => setAmount(e.target.value)} aria-label="额度 micro-USD" placeholder="amount_minor" />
          <Button size="sm" variant="outline" onClick={loadQuota}>
            读取额度
          </Button>
          <Button size="sm" onClick={grant}>
            调整额度
          </Button>
        </div>
        <p className="text-sm text-slate-300">{message}</p>
      </section>
      <form
        className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6"
        onSubmit={async (event) => {
          event.preventDefault();
          const form = event.currentTarget;
          const data = new FormData(form);
          const res = await fetch(`${apiBase}/admin/channels`, {
            method: "POST",
            credentials: "include",
            headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
            body: JSON.stringify({
              code: String(data.get("code") || "").trim(),
              type: String(data.get("type") || "B").trim() || "B",
              status: String(data.get("status") || "active").trim() || "active",
              brand_id: String(data.get("brand_id") || "").trim(),
            }),
          });
          const body = await res.json();
          if (!res.ok) {
            setChannelMessage(body.error?.message || "创建失败");
            return;
          }
          form.reset();
          setChannelMessage(`已创建 ${body.item?.id} ${body.item?.code} → ${body.item?.type} / ${body.item?.status}`);
          await queryClient.invalidateQueries();
        }}
      >
        <h2 className="mb-3 text-xl font-medium">创建渠道</h2>
        <p className="mb-3 text-sm text-slate-400">code 要唯一。类型 A/B/C。品牌默认官方站。创建后用户仍只能靠推广码归因，不能自助改渠道。</p>
        <div className="mb-3 grid max-w-xl gap-2">
          <Input name="code" aria-label="创建用渠道 code" placeholder="创建用渠道 code" />
          <Input name="type" aria-label="创建用渠道类型" placeholder="创建用渠道类型 B" defaultValue="B" />
          <Input name="status" aria-label="创建用渠道状态" placeholder="创建用渠道状态 active" defaultValue="active" />
          <Input name="brand_id" aria-label="创建用品牌 ID" placeholder="创建用品牌 ID brd_official" />
        </div>
        <Button size="sm" type="submit">
          创建渠道
        </Button>
      </form>
      <form
        className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6"
        onSubmit={async (event) => {
          event.preventDefault();
          const form = event.currentTarget;
          const data = new FormData(form);
          const id = String(data.get("channel_id") || "").trim();
          const res = await fetch(`${apiBase}/admin/channels/${id}`, {
            method: "PATCH",
            credentials: "include",
            headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
            body: JSON.stringify({
              status: String(data.get("status") || "").trim(),
              type: String(data.get("type") || "").trim(),
              brand_id: String(data.get("brand_id") || "").trim(),
            }),
          });
          const body = await res.json();
          if (!res.ok) {
            setChannelMessage(body.error?.message || "保存失败");
            return;
          }
          setChannelMessage(`已保存 ${body.item?.id} → ${body.item?.status} / ${body.item?.type}`);
          await queryClient.invalidateQueries();
        }}
      >
        <h2 className="mb-3 text-xl font-medium">改渠道状态</h2>
        <p className="mb-3 text-sm text-slate-400">只改状态、类型或品牌。停用后冻结新消费（聊天/媒体 403），余额和历史仍保留。不要停用官方/代理商/OEM 种子渠道。</p>
        <div className="mb-3 grid max-w-xl gap-2">
          <Input name="channel_id" aria-label="改状态用渠道 ID" placeholder="改状态用渠道 ID" />
          <Input name="status" aria-label="改状态用状态" placeholder="改状态用状态 disabled" />
          <Input name="type" aria-label="改状态用类型" placeholder="改状态用类型 B" />
          <Input name="brand_id" aria-label="改状态用品牌 ID" placeholder="改状态用品牌 ID" />
        </div>
        <Button size="sm" type="submit">
          保存渠道
        </Button>
      </form>
      <p className="text-sm text-slate-300">{channelMessage}</p>
      <AdminListPanel<Channel>
        path="/admin/channels"
        title="渠道 / 代理商"
        columns={[
          { accessorKey: "code", header: "Code" },
          { accessorKey: "type", header: "Type" },
          { accessorKey: "status", header: "Status" },
          { accessorKey: "brand_id", header: "Brand" },
        ]}
      />
    </AdminShell>
  );
}
