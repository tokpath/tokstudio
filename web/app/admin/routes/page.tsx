"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
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
import { IfCan } from "@/components/rbac/if-can";

type Route = { id: string; public_model_id: string; strategy: string; status: string };

const createSchema = z.object({
  public_model_id: z.string().trim().min(1, "请填写 public model id"),
  strategy: z.string().trim().min(1, "请填写策略"),
  status: z.string().trim().min(1, "请填写状态"),
  provider_id: z.string().trim(),
});

const patchSchema = z.object({
  route_id: z.string().trim().min(1, "请填写路由 id"),
  strategy: z.string().trim(),
  status: z.string().trim(),
});

export default function AdminRoutesPage() {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("创建和改策略都要二次确认。不要改 rg_echo，那是文本网关默认路由。");
  const createForm = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: { public_model_id: "", strategy: "priority", status: "active", provider_id: "" },
  });
  const patchForm = useForm<z.infer<typeof patchSchema>>({
    resolver: zodResolver(patchSchema),
    defaultValues: { route_id: "", strategy: "", status: "" },
  });

  return (
    <AdminShell>
      <AdminListPanel<Route>
        path="/admin/routes"
        title="路由组"
        columns={[
          { accessorKey: "id", header: "ID" },
          { accessorKey: "public_model_id", header: "Model" },
          { accessorKey: "strategy", header: "Strategy" },
          { accessorKey: "status", header: "Status" },
        ]}
      />
      <IfCan action="routes.write">
      <Form {...createForm}>
        <form className="mt-4 grid max-w-xl gap-2 rounded-card border border-hairline bg-canvas-raised  p-4" onSubmit={(event) => event.preventDefault()}>
          <AdminH2 k="createRoute" className="text-lg font-semibold tracking-tight" />
          <p className="text-sm text-ink-secondary">给已有公开模型建一个路由组。策略可选 priority / weight / price / health。</p>
          <TextField control={createForm.control} name="public_model_id" label="创建用 public model id" />
          <TextField control={createForm.control} name="strategy" label="创建用策略" placeholder="创建用策略 priority" />
          <TextField control={createForm.control} name="status" label="创建用状态" placeholder="创建用状态 active" />
          <TextField control={createForm.control} name="provider_id" label="创建用 provider id" placeholder="创建用 provider id（可选候选）" />
          <ConfirmButton
            size="sm"
            title="确认创建路由"
            description="不要改 rg_echo。策略可选 priority / weight / price / health。"
            validate={() => createForm.trigger()}
            onConfirm={createForm.handleSubmit(async (values) => {
              const body: Record<string, unknown> = {
                public_model_id: values.public_model_id,
                strategy: values.strategy || "priority",
                status: values.status || "active",
              };
              if (values.provider_id) {
                body.candidates = [{ provider_id: values.provider_id, priority: 1, weight: 1 }];
              }
              const res = await fetch(`${apiBase}/admin/routes`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: JSON.stringify(body),
              });
              const json = await res.json();
              if (!res.ok) {
                setMessage(json.error?.message || "创建失败");
                return;
              }
              createForm.reset();
              setMessage(`已创建 ${json.item?.id} → ${json.item?.strategy} / ${json.item?.status}`);
              await queryClient.invalidateQueries();
            })}
          >
            创建路由
          </ConfirmButton>
        </form>
      </Form>
      <Form {...patchForm}>
        <form className="mt-4 grid max-w-xl gap-2 rounded-card border border-hairline bg-canvas-raised  p-4" onSubmit={(event) => event.preventDefault()}>
          <AdminH2 k="editRoute" className="text-lg font-semibold tracking-tight" />
          <p className="text-sm text-ink-secondary">只改策略或状态。不要对 rg_echo 乱改，改完会改变 echo 网关的选路。</p>
          <TextField control={patchForm.control} name="route_id" label="改策略用路由 id" />
          <TextField control={patchForm.control} name="strategy" label="改策略用策略" placeholder="改策略用策略 health" />
          <TextField control={patchForm.control} name="status" label="改策略用状态" placeholder="改策略用状态 active" />
          <ConfirmButton
            size="sm"
            title="确认保存策略"
            description="不要对 rg_echo 乱改。"
            validate={() => patchForm.trigger()}
            onConfirm={patchForm.handleSubmit(async (values) => {
              const res = await fetch(`${apiBase}/admin/routes/${values.route_id}`, {
                method: "PATCH",
                credentials: "include",
                headers: confirmHeaders,
                body: JSON.stringify({ strategy: values.strategy, status: values.status }),
              });
              const json = await res.json();
              if (!res.ok) {
                setMessage(json.error?.message || "保存失败");
                return;
              }
              setMessage(`已保存 ${json.item?.id} → ${json.item?.strategy} / ${json.item?.status}`);
              await queryClient.invalidateQueries();
            })}
          >
            保存策略
          </ConfirmButton>
        </form>
      </Form>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </IfCan>
    </AdminShell>
  );
}
