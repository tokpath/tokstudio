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

type Key = { id: string; user_id: string; name: string; prefix: string; rpm_limit: number; status: string };

const schema = z.object({
  key_id: z.string().trim().min(1, "请填写 API Key ID"),
});

export default function AdminKeysPage() {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("禁用要二次确认。列表只有 prefix，没有完整密钥。不要禁正在跑 e2e 的主 Key。");
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { key_id: "" },
  });

  return (
    <AdminShell>
      <Form {...form}>
        <form className="rounded-2xl border border-white/10 bg-white/[0.035] shadow-glow p-6" onSubmit={(event) => event.preventDefault()}>
          <h2 className="mb-3 text-xl font-medium">禁用 API Key</h2>
          <p className="mb-3 text-sm text-slate-400">平台管理员和技术值班可以禁任意用户的 Key。禁用后网关立刻 403，不会回显密文。</p>
          <div className="mb-3 grid max-w-xl gap-2">
            <TextField control={form.control} name="key_id" label="禁用用 API Key ID" />
          </div>
          <ConfirmButton
            size="sm"
            title="确认禁用 Key"
            description="禁用后网关立刻 403。不要禁正在跑 e2e 的主 Key。"
            validate={() => form.trigger()}
            onConfirm={form.handleSubmit(async (values) => {
              const res = await fetch(`${apiBase}/admin/api-keys/${values.key_id}/disable`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: "{}",
              });
              const body = await res.json();
              if (!res.ok) {
                setMessage(body.error?.message || "禁用失败");
                return;
              }
              form.reset();
              setMessage(`已禁用 ${body.item?.id} → ${body.item?.status} / ${body.item?.prefix || ""}`);
              await queryClient.invalidateQueries();
            })}
          >
            禁用 Key
          </ConfirmButton>
          <p className="mt-3 text-sm text-slate-300">{message}</p>
        </form>
      </Form>
      <AdminListPanel<Key>
        path="/admin/api-keys"
        title="API Key"
        columns={[
          { accessorKey: "prefix", header: "Prefix" },
          { accessorKey: "name", header: "Name" },
          { accessorKey: "user_id", header: "User" },
          { accessorKey: "rpm_limit", header: "RPM" },
          { accessorKey: "status", header: "Status" },
        ]}
      />
    </AdminShell>
  );
}
