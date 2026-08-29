"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { apiBase } from "@/lib/api";

type DocsContext = {
  brand?: { name?: string; api_domain?: string };
  models?: string[];
  examples?: { curl?: string; python?: string; node?: string; messages?: string; video?: string };
  notes?: { auth?: string; errors?: string; rate_limit?: string; webhook?: string };
};

export default function ExamplesPanel() {
  const [docs, setDocs] = useState<DocsContext>({});
  const [message, setMessage] = useState("示例会带上当前品牌的 Base URL 和模型白名单，不会写入完整 API Key。");

  async function refresh() {
    const host = typeof window !== "undefined" ? window.location.host : "localhost";
    const response = await fetch(`${apiBase}/v1/public/docs-context?host=${encodeURIComponent(host)}`, {
      credentials: "include",
    });
    const body = (await response.json()) as DocsContext & { error?: { message?: string } };
    if (!response.ok) {
      setMessage(body.error?.message || "文档上下文加载失败");
      return;
    }
    setDocs(body);
    setMessage(`Base URL https://${body.brand?.api_domain || "localhost"} ，模型 ${(body.models || []).join("、") || "无"}`);
  }

  async function copy(label: string, text?: string) {
    if (!text) {
      setMessage("请先刷新接入示例");
      return;
    }
    await navigator.clipboard.writeText(text);
    setMessage(`已复制 ${label}，请自行替换 $TOKENHUB_API_KEY`);
  }

  return (
    <Card className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
      <CardTitle className="mb-3 text-xl font-medium">接入示例</CardTitle>
      <p className="mb-3 text-sm text-slate-400">
        覆盖 Chat、Anthropic Messages 和视频任务。完整 Key 只在「API Key」面板复制，不会出现在这段文档里。
      </p>
      <div className="mb-3 flex flex-wrap gap-3">
        <Button variant="outline" onClick={refresh}>
          刷新示例
        </Button>
        <Button variant="outline" onClick={() => copy("curl", docs.examples?.curl)}>
          复制 curl
        </Button>
        <Button variant="outline" onClick={() => copy("Python", docs.examples?.python)}>
          复制 Python
        </Button>
        <Button variant="outline" onClick={() => copy("Node.js", docs.examples?.node)}>
          复制 Node.js
        </Button>
      </div>
      <pre className="overflow-x-auto rounded bg-slate-950 p-3 text-xs text-cyan-100">{docs.examples?.curl || "点击刷新示例"}</pre>
      <p className="mt-3 text-sm text-slate-400">{docs.notes?.errors}</p>
      <p className="mt-1 text-sm text-slate-400">{docs.notes?.rate_limit}</p>
      <p className="mt-1 text-sm text-slate-400">{docs.notes?.webhook}</p>
      <p className="mt-3 text-sm text-slate-300">{message}</p>
    </Card>
  );
}
