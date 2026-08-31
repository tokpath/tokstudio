"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { PublicSection } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

const SCENES = [
  { id: "coding", label: "编程", hint: "推理与工具调用优先" },
  { id: "agent", label: "AI 智能体", hint: "长上下文 + 函数" },
  { id: "rag", label: "RAG / 长文档", hint: "大窗口更稳" },
  { id: "vision", label: "视觉", hint: "带 vision 能力" },
  { id: "cheap", label: "最便宜", hint: "看输入单价" },
  { id: "fast", label: "最快", hint: "轻量 flash 类" },
] as const;

const REC: Record<(typeof SCENES)[number]["id"], { title: string; why: string; href: string }[]> = {
  coding: [
    { title: "编程模型（价目白名单）", why: "优先选支持 tools / reasoning 的文本模型。", href: "/models" },
    { title: "Vibe Coding 接入", why: "Claude Code / Codex 只改 URL。", href: "/vibe-coding" },
  ],
  agent: [
    { title: "用户台路由回单", why: "Agent 失败要看 attempt。", href: "/app" },
    { title: "快速开始", why: "三步拿到 Key。", href: "/quickstart" },
  ],
  rag: [
    { title: "模型目录", why: "按上下文与单价筛。", href: "/models" },
    { title: "性价比表", why: "长文档看输入成本。", href: "/best-value" },
  ],
  vision: [
    { title: "图像模型", why: "图像生成与理解分区。", href: "/image" },
    { title: "全部模型", why: "筛能力标签。", href: "/models" },
  ],
  cheap: [{ title: "性价比模型", why: "按公开卖价排序浏览。", href: "/best-value" }],
  fast: [{ title: "快速开始", why: "先跑通再换模型。", href: "/quickstart" }],
};

export default function ModelFinderPage() {
  const [scene, setScene] = useState<(typeof SCENES)[number]["id"]>("coding");
  const recs = useMemo(() => REC[scene], [scene]);

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-12 px-6 py-20">
      <I18nPublicHero id="modelFinder" primaryHref="/login" secondaryHref="/models" />

      <PublicSection eyebrow="SCENE" title="你要做什么？">
        <div className="flex flex-wrap gap-2">
          {SCENES.map((s) => (
            <button
              key={s.id}
              type="button"
              onClick={() => setScene(s.id)}
              className={`rounded-control px-3 py-2 text-sm ${
                scene === s.id ? "bg-brand-soft text-brand-emphasis" : "border border-hairline text-ink-secondary"
              }`}
            >
              {s.label}
            </button>
          ))}
        </div>
        <p className="text-sm text-ink-mute">{SCENES.find((s) => s.id === scene)?.hint}</p>
      </PublicSection>

      <PublicSection eyebrow="RESULT" title="推荐入口">
        <ul className="space-y-3">
          {recs.map((r) => (
            <li key={r.href + r.title} className="rounded-card border border-hairline bg-canvas-raised p-5">
              <p className="font-semibold">{r.title}</p>
              <p className="mt-1 text-sm text-ink-secondary">{r.why}</p>
              <Button asChild variant="outline" size="sm" className="mt-3">
                <Link href={r.href}>打开</Link>
              </Button>
            </li>
          ))}
        </ul>
      </PublicSection>
    </main>
  );
}
