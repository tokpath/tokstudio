"use client";

import { useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Form } from "@/components/ui/form";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { AdminH2 } from "@/components/admin-h2";

type Role = { id: string; channel_org_id: string; type: string; parent_id?: string; status: string };
type Promo = { id: string; code: string; channel_org_id: string; acquisition_role_id?: string; status: string };

const roleSchema = z.object({
  channel_id: z.string().trim().min(1, "请填写渠道 ID"),
  role_type: z.string().trim().min(1, "请填写角色类型"),
  parent_id: z.string().trim(),
});

const promoSchema = z.object({
  channel_id: z.string().trim().min(1, "请填写渠道 ID"),
  role_id: z.string().trim().min(1, "请填写推广角色 ID"),
  code: z.string().trim(),
});

export default function AdminPromosPage() {
  const [message, setMessage] = useState("推广码在注册时固化归因。创建角色和推广码都要二次确认。");
  const roleForm = useForm<z.infer<typeof roleSchema>>({
    resolver: zodResolver(roleSchema),
    defaultValues: { channel_id: "chn_reseller_b", role_type: "kol_l2", parent_id: "acr_b_kol1" },
  });
  const promoForm = useForm<z.infer<typeof promoSchema>>({
    resolver: zodResolver(promoSchema),
    defaultValues: { channel_id: "chn_reseller_b", role_id: "acr_b_kol2", code: "" },
  });

  return (
    <AdminShell>
      <Form {...roleForm}>
        <form className="rounded-card border border-hairline bg-canvas-raised  p-6" onSubmit={(event) => event.preventDefault()}>
          <AdminH2 k="promoRoles" className="mb-3 text-xl font-medium" />
          <p className="mb-3 text-sm text-ink-secondary">层级只能是 agent → kol_l1 → kol_l2。2 级必须挂在 1 级下面。</p>
          <div className="mb-3 flex flex-wrap items-end gap-2">
            <TextField control={roleForm.control} name="channel_id" label="渠道 ID" showLabel={false} className="w-48" />
            <TextField control={roleForm.control} name="role_type" label="角色类型" showLabel={false} className="w-28" />
            <TextField control={roleForm.control} name="parent_id" label="上级角色" showLabel={false} className="w-48" />
            <ConfirmButton
              size="sm"
              title="确认创建推广角色"
              description="层级只能是 agent → kol_l1 → kol_l2。"
              validate={() => roleForm.trigger()}
              onConfirm={roleForm.handleSubmit(async (values) => {
                const res = await fetch(`${apiBase}/admin/acquisition-roles`, {
                  method: "POST",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify({ channel_org_id: values.channel_id, type: values.role_type, parent_id: values.parent_id }),
                });
                const body = await res.json();
                if (!res.ok) {
                  setMessage(body.error?.message || "创建角色失败");
                  return;
                }
                if (body.item?.id) {
                  promoForm.setValue("role_id", body.item.id);
                  promoForm.setValue("channel_id", values.channel_id);
                }
                setMessage(`已创建角色 ${body.item?.id}（${body.item?.type}）`);
              })}
            >
              创建推广角色
            </ConfirmButton>
          </div>
          <p className="text-sm text-ink-secondary">{message}</p>
        </form>
      </Form>
      <Form {...promoForm}>
        <form className="rounded-card border border-hairline bg-canvas-raised  p-6" onSubmit={(event) => event.preventDefault()}>
          <AdminH2 k="promos" className="mb-3 text-xl font-medium" />
          <p className="mb-3 text-sm text-ink-secondary">把码发给用户，或复制 /login?promo=CODE。服务端按码反查渠道和角色。</p>
          <div className="mb-3 flex flex-wrap items-end gap-2">
            <TextField control={promoForm.control} name="role_id" label="推广角色 ID" showLabel={false} className="w-48" />
            <TextField control={promoForm.control} name="code" label="推广码" placeholder="THB-SALE" showLabel={false} className="w-40" />
            <ConfirmButton
              size="sm"
              title="确认创建推广码"
              description="注册时会按这个码固化渠道和角色。"
              validate={() => promoForm.trigger()}
              onConfirm={promoForm.handleSubmit(async (values) => {
                const res = await fetch(`${apiBase}/admin/promotion-codes`, {
                  method: "POST",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify({
                    channel_org_id: values.channel_id,
                    acquisition_role_id: values.role_id,
                    code: values.code,
                  }),
                });
                const body = await res.json();
                setMessage(res.ok ? `已创建推广码 ${body.item?.code}` : body.error?.message || "创建推广码失败");
              })}
            >
              创建推广码
            </ConfirmButton>
          </div>
        </form>
      </Form>
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
