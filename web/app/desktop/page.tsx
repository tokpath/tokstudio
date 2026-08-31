import Link from "next/link";
import { Button } from "@/components/ui/button";
import { PublicSection, StatStrip } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

const TOOLS = [
  { name: "Claude Code", hint: "ANTHROPIC_BASE_URL" },
  { name: "Codex", hint: "~/.codex/config.toml" },
  { name: "Gemini CLI", hint: "自定义端点" },
  { name: "OpenCode", hint: "内置服务商" },
  { name: "Cline", hint: "OpenAI 兼容" },
  { name: "OpenClaw", hint: "改 base URL" },
];

const CAPS = [
  {
    t: "本机工具，同一份账",
    d: "Claude Code、Codex、Gemini CLI 等只要改 base URL，Key 和余额都在用户台。",
  },
  {
    t: "每个工具一把 Key",
    d: "在用户台为每个客户端建独立 API Key，单独限额、单独轮换，互不影响。",
  },
  {
    t: "模型自己指定",
    d: "Codex 跑 Qwen、Claude Code 跑 Fable，价目公开，不把平台成本写进客户端。",
  },
  {
    t: "用量在账本里",
    d: "余额、HOLD、请求明细都在用户台。客户端只负责发请求，不算第二套账单。",
  },
];

export default function DesktopPage() {
  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-12 px-6 py-20">
      <I18nPublicHero id="desktop" primaryHref="/vibe-coding" secondaryHref="/login" />

      <StatStrip
        items={[
          { label: "安装包", value: "未发布", hint: "不提供假下载" },
          { label: "当前路径", value: "Web", hint: "改 base URL" },
          { label: "账单", value: "一本", hint: "用户台账本" },
        ]}
      />

      <PublicSection eyebrow="TOOLS" title="支持的工具" description="凡是允许自定义 API 端点的客户端都能接。下面是已经写过片段的几个。">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {TOOLS.map((tool) => (
            <div key={tool.name} className="rounded-card border border-hairline bg-canvas-raised p-4">
              <p className="font-medium text-ink">{tool.name}</p>
              <p className="mt-1 font-mono text-[13px] text-ink-mute">{tool.hint}</p>
            </div>
          ))}
        </div>
      </PublicSection>

      <PublicSection eyebrow="CAPABILITY" title="告别多份 Key 和多份账单">
        <div className="grid gap-3 md:grid-cols-2">
          {CAPS.map((cap) => (
            <div key={cap.t} className="rounded-card border border-hairline bg-canvas-raised p-5">
              <h3 className="text-lg font-semibold">{cap.t}</h3>
              <p className="mt-2 text-sm text-ink-secondary">{cap.d}</p>
            </div>
          ))}
        </div>
      </PublicSection>

      <PublicSection eyebrow="STATUS" title="当前可用">
        <p className="text-sm text-ink-secondary">
          桌面安装包尚未随本仓库发布。请用浏览器打开公共站与用户台，或在 Claude Code / Codex 里改 base URL。Intel / Windows 客户端也不假装「正在路上」。
        </p>
        <div className="mt-4 flex flex-wrap gap-3">
          <Button asChild>
            <Link href="/quickstart">Web 快速开始</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/vibe-coding">Vibe Coding</Link>
          </Button>
        </div>
      </PublicSection>
    </main>
  );
}
