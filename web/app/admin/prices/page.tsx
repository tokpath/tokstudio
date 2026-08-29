"use client";

import { useState } from "react";
import { useForm } from "react-hook-form";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";

type PriceBook = { id: string; public_id: string; status: string };

export default function AdminPricesPage() {
  const form = useForm({ defaultValues: { model: "tokenhub/echo-1", input: "0.000001", output: "0.000002" } });
  const [message, setMessage] = useState("新价格只影响之后的请求，旧账单保持快照。发布需要二次确认。");

  return (
    <AdminShell>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-3 text-xl font-medium">价格</h2>
        <p className="mb-3 text-sm text-slate-400">发布新版本会把当前 published 标成 superseded，历史 usage 仍按当时快照计费。</p>
        <form
          className="mb-3 flex flex-wrap gap-2"
          onSubmit={form.handleSubmit(async (values) => {
            const res = await fetch(`${apiBase}/admin/price-books`, {
              method: "POST",
              credentials: "include",
              headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
              body: JSON.stringify({ model: values.model, input: values.input, output: values.output, currency: "USD" }),
            });
            const body = await res.json();
            setMessage(res.ok ? `已发布 ${body.price?.PublicID || values.model}` : body.error?.message || "发布失败");
          })}
        >
          <Input className="w-56" placeholder="public model id" {...form.register("model")} aria-label="模型 ID" />
          <Input className="w-32" placeholder="input" {...form.register("input")} aria-label="输入单价" />
          <Input className="w-32" placeholder="output" {...form.register("output")} aria-label="输出单价" />
          <Button type="submit" size="sm">
            发布价格
          </Button>
        </form>
        <p className="text-sm text-slate-300">{message}</p>
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
