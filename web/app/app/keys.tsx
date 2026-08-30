"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { useToast } from "@/lib/toast";

export type APIKeyItem = {
  id: string;
  name: string;
  prefix: string;
  key?: string;
  status: string;
  rpm_limit?: number;
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
    return <p className="text-sm text-slate-400">还没有 API Key。</p>;
  }
  return (
    <ul className="space-y-3 text-sm text-slate-200">
      {items.map((item) => (
        <li key={item.id} className="rounded border border-slate-800 p-3">
          <p>
            {item.name} · {item.prefix} · {item.status}
            {item.rpm_limit ? ` · RPM ${item.rpm_limit}` : ""}
          </p>
          <p className="text-slate-400">
            模型白名单：{item.allowlist?.length ? item.allowlist.join(", ") : "不限制"}
          </p>
          {item.key ? <p className="break-all text-slate-400">{item.key}</p> : null}
        </li>
      ))}
    </ul>
  );
}

export default function KeysPanel() {
  const [items, setItems] = useState<APIKeyItem[]>([]);
  const [name, setName] = useState("default");
  const [allowlist, setAllowlist] = useState("");
  const [rpm, setRpm] = useState("");
  const [createMessage, setCreateMessage] = useState("空白名单不限制模型；填了之后，不在名单里的模型会返回 403 model_not_allowed。");
  const message = useToast((s) => s.message);
  const setMessage = useToast((s) => s.setMessage);

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

  async function createKey() {
    const models = parseAllowlist(allowlist);
    const payload: { name: string; allowlist?: string[]; rpm_limit?: number } = { name };
    if (models.length > 0) {
      payload.allowlist = models;
    }
    const rpmLimit = Number(rpm);
    if (rpm && Number.isFinite(rpmLimit) && rpmLimit > 0) {
      payload.rpm_limit = rpmLimit;
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
    setCreateMessage(`已创建 ${created.id || ""} ${created.name || name} → 白名单 ${listed}`);
    setAllowlist("");
    setRpm("");
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
      <p className="mb-4 text-sm text-slate-400">
        完整 Key 可长期查看。轮换、复制、禁用、过期都会写审计日志；过期或禁用后网关返回 403。
      </p>
      <div className="mb-4 grid max-w-xl gap-3">
        <label className="text-sm text-slate-400" htmlFor="api-key-name">
          Key 名称
        </label>
        <Input id="api-key-name" value={name} aria-label="API Key 名称" placeholder="Key 名称" onChange={(e) => setName(e.target.value)} />
        <h3 className="text-lg font-medium">模型白名单</h3>
        <Input
          id="api-key-allowlist"
          value={allowlist}
          aria-label="模型白名单"
          placeholder="逗号分隔模型，空则不限制"
          onChange={(e) => setAllowlist(e.target.value)}
        />
        <Input
          id="api-key-rpm"
          value={rpm}
          aria-label="RPM 限额"
          placeholder="可选 RPM，默认 60"
          onChange={(e) => setRpm(e.target.value)}
        />
        <div className="flex flex-wrap gap-3">
          <Button onClick={createKey}>创建</Button>
          <Button variant="outline" onClick={refresh}>
            刷新
          </Button>
        </div>
        <p className="text-sm text-slate-300">{createMessage}</p>
      </div>
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
      <p className="mt-3 text-sm text-slate-300">{message}</p>
    </Card>
  );
}
