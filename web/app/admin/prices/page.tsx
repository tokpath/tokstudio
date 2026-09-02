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
import { IfCan } from "@/components/rbac/if-can";

type PriceBook = { id: string; public_id: string; status: string };

const schema = z.object({
  model: z.string().trim().min(1, "请填写模型 ID"),
  input: z.string().trim().min(1, "请填写输入单价"),
  output: z.string().trim().min(1, "请填写输出单价"),
});

export default function AdminPricesPage() {
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { model: "tokenhub/echo-1", input: "0.000001", output: "0.000002" },
  });
  const [message, setMessage] = useState("新价格只影响之后的请求，旧账单保持快照。发布需要二次确认。");

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="prices" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">发布新版本会把当前 published 标成 superseded，历史 usage 仍按当时快照计费。单个模型也可以在模型编辑页改价。</p>
        <IfCan action="prices.write">
        <Form {...form}>
          <form className="mb-3 flex flex-wrap items-end gap-2" onSubmit={(event) => event.preventDefault()}>
            <TextField control={form.control} name="model" label="模型 ID" placeholder="public model id" showLabel={false} className="w-56" />
            <TextField control={form.control} name="input" label="输入单价" placeholder="input" showLabel={false} className="w-32" />
            <TextField control={form.control} name="output" label="输出单价" placeholder="output" showLabel={false} className="w-32" />
            <ConfirmButton
              size="sm"
              title="确认发布价格"
              description="新价格只影响之后的请求，旧账单保持快照。"
              validate={() => form.trigger()}
              onConfirm={form.handleSubmit(async (values) => {
                const res = await fetch(`${apiBase}/admin/price-books`, {
                  method: "POST",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify({ model: values.model, input: values.input, output: values.output, currency: "USD" }),
                });
                const body = await res.json();
                setMessage(res.ok ? `已发布 ${body.price?.PublicID || values.model}` : body.error?.message || "发布失败");
              })}
            >
              发布价格
            </ConfirmButton>
          </form>
        </Form>
        </IfCan>
        <p className="text-sm text-ink-secondary">{message}</p>
      </section>
      <AdminListPanel<PriceBook>
        path="/admin/price-books"
        title="价格版本"
        columns={[
          { accessorKey: "public_id", header: "Model" },
          { accessorKey: "status", header: "Status" },
          { accessorKey: "id", header: "Version" },
        ]}
      />
    </AdminShell>
  );
}
