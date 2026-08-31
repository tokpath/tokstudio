"use client";

import { useState } from "react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/code-block";

const PROMPTS = [
  "用两段话解释这个模型的独特之处",
  "写一首关于大语言模型的俳句",
  "总结微服务架构的优缺点",
];

/**
 * 模型详情 Playground：结构对齐 ofox（提示词 / 输入 / 登录后发送）。
 * 未登录不伪造上游回复；登录后引导用户台。皮肤走 DESIGN.md。
 */
export function ModelPlayground({
  modelId,
  displayName,
  kind,
  sampleCurl,
}: {
  modelId: string;
  displayName: string;
  kind: string;
  sampleCurl: string;
}) {
  const [tab, setTab] = useState<"chat" | "code">("chat");
  const [input, setInput] = useState("");
  const [lang, setLang] = useState<"curl" | "python" | "node">("curl");
  const [note, setNote] = useState("对话不会被保存——离开或刷新页面后即清空。");

  function usePrompt(p: string) {
    setInput(p);
    setTab("chat");
  }

  function onSubmit() {
    if (!input.trim()) {
      setNote("请先输入内容。");
      return;
    }
    setNote("未登录不能调用上游。请登录后在用户台发起请求；此处只演示结构。");
  }

  const code =
    lang === "python"
      ? `from openai import OpenAI\nclient = OpenAI(base_url="https://api.tokenhub.local/v1", api_key="sk-...xxxx")\nr = client.chat.completions.create(\n  model="${modelId}",\n  messages=[{"role":"user","content":${JSON.stringify(input || "hello")}}],\n)\nprint(r.choices[0].message.content)`
      : lang === "node"
        ? `import OpenAI from "openai";\nconst client = new OpenAI({ baseURL: "https://api.tokenhub.local/v1", apiKey: "sk-...xxxx" });\nconst r = await client.chat.completions.create({\n  model: "${modelId}",\n  messages: [{ role: "user", content: ${JSON.stringify(input || "hello")} }],\n});\nconsole.log(r.choices[0].message.content);`
        : sampleCurl;

  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p className="th-eyebrow text-ink-mute">PLAYGROUND</p>
          <h2 className="mt-1 text-lg font-semibold">试用 {displayName}</h2>
        </div>
        <div className="flex gap-1">
          {(
            [
              ["chat", "对话"],
              ["code", "代码"],
            ] as const
          ).map(([id, label]) => (
            <button
              key={id}
              type="button"
              onClick={() => setTab(id)}
              className={`rounded-control px-3 py-1.5 text-sm ${
                tab === id ? "bg-brand-soft text-brand-emphasis" : "text-ink-mute"
              }`}
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      <p className="mt-2 text-[13px] text-ink-mute">{note}</p>

      {tab === "chat" ? (
        <div className="mt-4 flex flex-col gap-3">
          <div className="flex flex-wrap gap-2">
            {PROMPTS.map((p) => (
              <button
                key={p}
                type="button"
                onClick={() => usePrompt(p)}
                className="rounded-control border border-hairline px-3 py-1.5 text-left text-[13px] text-ink-secondary hover:bg-brand-soft/40"
              >
                {p}
              </button>
            ))}
          </div>
          <label className="text-sm text-ink-secondary" htmlFor="pg-input">
            向 {displayName} 发送消息
          </label>
          <textarea
            id="pg-input"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            rows={4}
            placeholder={`向 ${displayName} 发送消息…`}
            className="w-full rounded-control border border-hairline bg-canvas px-3 py-2 text-sm text-ink placeholder:text-ink-mute focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand-emphasis"
          />
          <div className="flex flex-wrap gap-2">
            <Button type="button" onClick={onSubmit}>
              发送请求
            </Button>
            <Button asChild type="button" variant="outline">
              <Link href="/login">登录后试用</Link>
            </Button>
            <span className="self-center text-[12px] text-ink-mute">模态 · {kind}</span>
          </div>
        </div>
      ) : (
        <div className="mt-4 flex flex-col gap-3">
          <div className="flex gap-1">
            {(
              [
                ["curl", "cURL"],
                ["python", "Python"],
                ["node", "Node.js"],
              ] as const
            ).map(([id, label]) => (
              <button
                key={id}
                type="button"
                onClick={() => setLang(id)}
                className={`rounded-control px-3 py-1.5 text-sm ${
                  lang === id ? "bg-brand-soft text-brand-emphasis" : "border border-hairline text-ink-mute"
                }`}
              >
                {label}
              </button>
            ))}
          </div>
          <CodeBlock>{code}</CodeBlock>
        </div>
      )}
    </section>
  );
}
