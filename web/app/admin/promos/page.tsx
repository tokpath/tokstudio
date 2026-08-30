"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";

type Role = { id: string; channel_org_id: string; type: string; parent_id?: string; status: string };
type Promo = { id: string; code: string; channel_org_id: string; acquisition_role_id?: string; status: string };

export default function AdminPromosPage() {
  const [channelID, setChannelID] = useState("chn_reseller_b");
  const [roleType, setRoleType] = useState("kol_l2");
  const [parentID, setParentID] = useState("acr_b_kol1");
  const [roleID, setRoleID] = useState("acr_b_kol2");
  const [code, setCode] = useState("");
  const [message, setMessage] = useState("推广码在注册时固化归因。创建角色和推广码都要二次确认。");

  async function createRole() {
    const res = await fetch(`${apiBase}/admin/acquisition-roles`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: JSON.stringify({ channel_org_id: channelID, type: roleType, parent_id: parentID }),
    });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "创建角色失败");
      return;
    }
    if (body.item?.id) {
      setRoleID(body.item.id);
    }
    setMessage(`已创建角色 ${body.item?.id}（${body.item?.type}）`);
  }

  async function createPromo() {
    const res = await fetch(`${apiBase}/admin/promotion-codes`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: JSON.stringify({ channel_org_id: channelID, acquisition_role_id: roleID, code }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已创建推广码 ${body.item?.code}` : body.error?.message || "创建推广码失败");
  }

  return (
    <AdminShell>
      <section className="rounded-2xl border border-white/10 bg-white/[0.035] shadow-glow p-6">
        <h2 className="mb-3 text-xl font-medium">推广角色</h2>
        <p className="mb-3 text-sm text-slate-400">层级只能是 agent → kol_l1 → kol_l2。2 级必须挂在 1 级下面。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-48" value={channelID} onChange={(e) => setChannelID(e.target.value)} aria-label="渠道 ID" />
          <Input className="w-28" value={roleType} onChange={(e) => setRoleType(e.target.value)} aria-label="角色类型" />
          <Input className="w-48" value={parentID} onChange={(e) => setParentID(e.target.value)} aria-label="上级角色" />
          <Button size="sm" onClick={createRole}>
            创建推广角色
          </Button>
        </div>
        <p className="text-sm text-slate-300">{message}</p>
      </section>
      <section className="rounded-2xl border border-white/10 bg-white/[0.035] shadow-glow p-6">
        <h2 className="mb-3 text-xl font-medium">推广码</h2>
        <p className="mb-3 text-sm text-slate-400">把码发给用户，或复制 /login?promo=CODE。服务端按码反查渠道和角色。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-48" value={roleID} onChange={(e) => setRoleID(e.target.value)} aria-label="推广角色 ID" />
          <Input className="w-40" value={code} onChange={(e) => setCode(e.target.value)} aria-label="推广码" placeholder="THB-SALE" />
          <Button size="sm" onClick={createPromo}>
            创建推广码
          </Button>
        </div>
      </section>
      <AdminListPanel<Role>
        path="/admin/acquisition-roles"
        title="角色列表"
        columns={[
          { accessorKey: "id", header: "ID" },
          { accessorKey: "type", header: "Type" },
          { accessorKey: "channel_org_id", header: "Channel" },
          { accessorKey: "status", header: "Status" },
        ]}
      />
      <AdminListPanel<Promo>
        path="/admin/promotion-codes"
        title="推广码列表"
        columns={[
          { accessorKey: "code", header: "Code" },
          { accessorKey: "channel_org_id", header: "Channel" },
          { accessorKey: "acquisition_role_id", header: "Role" },
          { accessorKey: "status", header: "Status" },
        ]}
      />
    </AdminShell>
  );
}
