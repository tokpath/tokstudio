"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { AdminSelectField } from "@/components/admin-select-field";
import { ConfirmButton, confirmFormSubmit } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Form } from "@/components/ui/form";
import { IfCan } from "@/components/rbac/if-can";
import { AdminShell } from "../../shell";
import { apiBase } from "@/lib/api";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { protocolOptions, providerHref } from "@/lib/catalog-admin";

const schema = z.object({
  name: z.string().trim().min(1, "请填写名称"),
  kind: z.enum(["direct", "aggregator"]),
  adapter: z.string().trim().min(1, "请选择协议"),
  base_url: z.string(),
});
type Fields = z.infer<typeof schema>;

export default function NewProviderPage() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("");
  const form = useForm<Fields>({
    resolver: zodResolver(schema),
    defaultValues: { name: "", kind: "direct", adapter: "openai", base_url: "" },
  });

  return <AdminShell>
    <Link className="text-sm text-brand-emphasis hover:underline" href="/admin/providers">返回提供商列表</Link>
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <h2 className="text-lg font-semibold">新建提供商</h2>
      <p className="mt-1 text-sm text-ink-secondary">配置上游连接。创建后在同一详情维护密钥、账号和健康状态。</p>
    </section>
    <IfCan action="providers.write">
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <Form {...form}>
          <form className="grid max-w-xl gap-4" onSubmit={(event) => event.preventDefault()}>
            <TextField control={form.control} name="name" label="名称" placeholder="例如 OpenAI" />
            <AdminSelectField control={form.control} name="kind" label="类型" options={[{ value: "direct", label: "直连" }, { value: "aggregator", label: "聚合" }]} />
            <AdminSelectField control={form.control} name="adapter" label="协议" options={protocolOptions(form.watch("adapter"))} />
            <TextField control={form.control} name="base_url" label="上游地址" placeholder="https://api.example.com/v1" />
            <ConfirmButton
              size="sm" title="确认创建提供商" description="创建连接后可在详情页添加密钥和账号。"
              validate={() => form.trigger()}
              onConfirm={confirmFormSubmit(form.handleSubmit, async (values) => {
                try {
                  const res = await fetch(`${apiBase}/admin/providers`, {
                    method: "POST", credentials: "include", headers: confirmHeaders, body: JSON.stringify(values),
                  });
                  const body = await res.json();
                  if (!res.ok) { setMessage(body.error?.message || "创建失败"); return false; }
                  await queryClient.invalidateQueries({ queryKey: ["/admin/providers"] });
                  router.replace(providerHref(String(body.item?.id || body.item?.slug)));
                  return true;
                } catch { setMessage(confirmNetworkUnavailable); return false; }
              })}
            >创建提供商</ConfirmButton>
            {message ? <p className="text-sm text-ink-secondary">{message}</p> : null}
          </form>
        </Form>
      </section>
    </IfCan>
  </AdminShell>;
}
