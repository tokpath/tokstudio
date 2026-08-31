"use client";

import { useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { apiBase } from "@/lib/api";
import type { CatalogModel } from "@/lib/catalog";

export function PlaygroundClient({ models }: { models: CatalogModel[] }) {
  const fallbackId = models[0]?.id || "tokenhub/echo-1";
  const [model, setModel] = useState(fallbackId);
  const [prompt, setPrompt] = useState("");
  const [output, setOutput] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("登录后可用会话调用；未登录会返回未授权。");

  const options = useMemo(() => (models.length ? models : [{ id: fallbackId, display_name: fallbackId, vendor: "" }]), [models, fallbackId]);

  async function send() {
    const text = prompt.trim();
    if (!text) {
      setMessage("请先写一条消息");
      return;
    }
    setBusy(true);
    setMessage("发送中…");
    try {
      const response = await fetch(`${apiBase}/v1/chat/completions`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model,
          messages: [{ role: "user", content: text }],
        }),
      });
      const body = await response.json();
      if (!response.ok) {
        setOutput("");
        setMessage(body.error?.message || `请求失败 ${response.status}`);
        return;
      }
      const content =
        body.choices?.[0]?.message?.content ||
        body.output_text ||
        JSON.stringify(body, null, 2);
      setOutput(typeof content === "string" ? content : JSON.stringify(content, null, 2));
      setMessage("已返回");
    } catch (err) {
      setMessage(err instanceof Error ? err.message : "网络错误");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
      <section className="flex flex-col gap-3 rounded-card border border-hairline bg-canvas-raised p-5">
        <label className="flex flex-col gap-1 text-xs text-ink-mute">
          模型
          <select
            aria-label="试用模型"
            className="h-10 rounded-control border border-hairline bg-canvas px-3 text-sm text-ink"
            value={model}
            onChange={(e) => setModel(e.target.value)}
          >
            {options.map((item) => (
              <option key={item.id} value={item.id}>
                {item.display_name || item.id}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs text-ink-mute">
          消息
          <textarea
            aria-label="试用消息"
            className="min-h-40 rounded-control border border-hairline bg-canvas px-3 py-2 text-sm text-ink"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="写一句，走网关试一条回单。"
          />
        </label>
        <div className="flex flex-wrap gap-2">
          <Button type="button" disabled={busy} onClick={() => void send()}>
            发送
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setPrompt("");
              setOutput("");
              setMessage("已清空");
            }}
          >
            清空
          </Button>
        </div>
        <p className="text-sm text-ink-secondary">{message}</p>
      </section>
      <section className="rounded-card border border-hairline bg-canvas-raised p-5">
        <h2 className="mb-3 text-lg font-semibold">回复</h2>
        {output ? (
          <pre className="th-scrollbar max-h-[420px] overflow-auto whitespace-pre-wrap font-mono text-sm text-ink">{output}</pre>
        ) : (
          <EmptyLedger title="发送一条消息开始试用" detail="没有演示对白。空着就是空着。" />
        )}
      </section>
    </div>
  );
}
