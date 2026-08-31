"use client";

import { useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Form } from "@/components/ui/form";
import { apiBase } from "@/lib/api";
import { useToast } from "@/lib/toast";

export type APIKeyItem = {
  id: string;
  name: string;
  prefix: string;
  key?: string;
  status: string;
  rpm_limit?: number;
  concurrency_limit?: number;
  allowlist?: string[];
  expires_at?: string | null;
  last_used_at?: string | null;
};

export function parseAllowlist(raw: string): string[] {
  return raw
    .split(/[,，]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export function KeysList({ items }: { items: APIKeyItem[] }) {
  if (items.length === 0) {
    return <EmptyLedger title="暂无 API Keys" detail="创建一把 Key 后会出现在这里。空白名单不限制模型。" />;
  }
  return (
    <ul className="space-y-3 text-sm text-ink">
      {items.map((item) => (
        <li key={item.id} className="rounded-card border border-hairline bg-canvas p-3">
          <p>
            {item.name} · {item.prefix} · {item.status}
            {item.rpm_limit ? ` · RPM ${item.rpm_limit}` : ""}
            {item.concurrency_limit ? ` · 并发 ${item.concurrency_limit}` : ""}
          </p>
          <p className="text-ink-secondary">
            模型白名单：{item.allowlist?.length ? item.allowlist.join(", ") : "不限制"}
          </p>
          {item.key ? <p className="break-all text-ink-secondary">{item.key}</p> : null}
        </li>
      ))}
    </ul>
  );
}

const createSchema = z.object({
  name: z.string().trim().min(1, "请填写 Key 名称"),
  allowlist: z.string(),
  rpm: z.string(),
  concurrency: z.string(),
});

export default function KeysPanel() {
  const [items, setItems] = useState<APIKeyItem[]>([]);
  const [createMessage, setCreateMessage] = useState("空白名单不限制模型；填了之后，不在名单里的模型会返回 403 model_not_allowed。RPM 默认 60，并发默认 5。");
  const message = useToast((s) => s.message);
  const setMessage = useToast((s) => s.setMessage);
  const form = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: { name: "default", allowlist: "", rpm: "", concurrency: "" },
  });

  async function refresh() {
    const response = await fetch(`${apiBase}/v1/me/api-keys`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "未登录");
      return;
    }
    setItems(body.items || []);
    setMessage("API Key 已刷新");
  }

  async function createKey(values: z.infer<typeof createSchema>) {
    const models = parseAllowlist(values.allowlist);
    const payload: { name: string; allowlist?: string[]; rpm_limit?: number; concurrency_limit?: number } = { name: values.name };
    if (models.length > 0) {
      payload.allowlist = models;
    }
    const rpmLimit = Number(values.rpm);
    if (values.rpm && Number.isFinite(rpmLimit) && rpmLimit > 0) {
      payload.rpm_limit = rpmLimit;
    }
    const concLimit = Number(values.concurrency);
    if (values.concurrency && Number.isFinite(concLimit) && concLimit > 0) {
      payload.concurrency_limit = concLimit;
    }
    const response = await fetch(`${apiBase}/v1/me/api-keys`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    const body = await response.json();
    if (!response.ok) {
      setCreateMessage(body.error?.message || "创建失败");
      return;
    }
    const created = body.item || {};
    const listed = Array.isArray(created.allowlist) && created.allowlist.length > 0 ? created.allowlist.join(", ") : "不限制";
    setCreateMessage(`已创建 ${created.id || ""} ${created.name || values.name} → 白名单 ${listed} / 并发 ${created.concurrency_limit || 5}`);
    form.reset({ name: "default", allowlist: "", rpm: "", concurrency: "" });
    await refresh();
  }

  async function act(id: string, action: "rotate" | "disable" | "expire" | "copy") {
    const response = await fetch(`${apiBase}/v1/me/api-keys/${id}/${action}`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: action === "expire" ? JSON.stringify({ expires_at: new Date().toISOString() }) : "{}",
    });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "操作失败");
      return;
    }
    if (action === "copy" && typeof navigator !== "undefined") {
      const secret = items.find((item) => item.id === id)?.key;
      if (secret) {
        await navigator.clipboard.writeText(secret);
      }
    }
    setMessage(action === "copy" ? "已复制并写入审计" : `已${action}`);
    await refresh();
  }

  return (
    <Card>
      <CardTitle>API Key</CardTitle>
      <p className="mb-4 text-sm text-ink-secondary">
        完整 Key 可长期查看。轮换、复制、禁用、过期都会写审计日志；过期或禁用后网关返回 403。
      </p>
      <Form {...form}>
        <form className="mb-4 grid max-w-xl gap-3" onSubmit={form.handleSubmit(createKey)}>
          <TextField control={form.control} name="name" label="API Key 名称" placeholder="Key 名称" />
          <h3 className="text-lg font-medium">模型白名单</h3>
          <TextField control={form.control} name="allowlist" label="模型白名单" placeholder="逗号分隔模型，空则不限制" />
          <TextField control={form.control} name="rpm" label="RPM 限额" placeholder="可选 RPM，默认 60" />
          <TextField control={form.control} name="concurrency" label="并发限额" placeholder="可选并发，默认 5" />
          <div className="flex flex-wrap gap-3">
            <Button type="submit">创建</Button>
            <Button type="button" variant="outline" onClick={refresh}>
              刷新
            </Button>
          </div>
          <p className="text-sm text-ink-secondary">{createMessage}</p>
        </form>
      </Form>
      <KeysList items={items} />
      <ul className="mt-4 space-y-2 text-sm">
        {items.map((item) => (
          <li key={`${item.id}-actions`} className="flex flex-wrap gap-2">
            <Button size="sm" variant="outline" onClick={() => act(item.id, "copy")}>
              复制
            </Button>
            <Button size="sm" variant="outline" onClick={() => act(item.id, "rotate")}>
              轮换
            </Button>
            <Button size="sm" variant="outline" onClick={() => act(item.id, "disable")}>
              禁用
            </Button>
            <Button size="sm" variant="outline" onClick={() => act(item.id, "expire")}>
              过期
            </Button>
          </li>
        ))}
      </ul>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
