import fixture from "@/lib/fixtures/ofox-catalog.json";
import { fetchAPI } from "@/lib/api";

export type CatalogModel = {
  id: string;
  vendor: string;
  display_name: string;
  capabilities?: Record<string, unknown>;
  sell_price?: Record<string, unknown>;
  status?: string;
  description?: string;
  context_length?: number;
  max_completion_tokens?: number;
  kind?: "text" | "image" | "video" | "embedding" | "audio" | string;
  created?: number;
};

const FALLBACK = fixture as CatalogModel[];

/** API 有白名单时优先用；否则用 ofox 公开目录快照，保证复刻页有完整密度。 */
export async function loadCatalog(host: string): Promise<CatalogModel[]> {
  try {
    const data = await fetchAPI<{ items: CatalogModel[] }>("/v1/public/models", { host });
    if (data.items?.length) {
      return data.items.map((m) => ({
        id: m.id || "unknown",
        vendor: m.vendor || "unknown",
        display_name: m.display_name || m.id || "unknown",
        capabilities: m.capabilities,
        sell_price: m.sell_price,
        status: m.status || "available",
        kind: m.kind || inferKind(m),
        description: m.description,
        context_length: m.context_length,
        max_completion_tokens: m.max_completion_tokens,
      }));
    }
  } catch {
    /* fall through */
  }
  return FALLBACK;
}

export function inferKind(m: Partial<CatalogModel>): string {
  if (m.kind) return m.kind;
  const id = (m.id || "").toLowerCase();
  const name = (m.display_name || "").toLowerCase();
  if (/(seedance|wan-|happyhorse|video)/.test(id + name)) return "video";
  if (/(image|seedream|flux|banana|dall)/.test(id + name)) return "image";
  if (/embed/.test(id)) return "embedding";
  if (/transcri|whisper|audio/.test(id)) return "audio";
  return "text";
}

export function formatMoney(raw: unknown, unit: "/M" | "/秒" | "/张" | "" = "/M") {
  if (raw == null || raw === "") return "—";
  const n = Number(raw);
  if (!Number.isFinite(n)) return String(raw);
  if (n === 0) return "$0";
  // ofox / OpenRouter 风格：prompt 常为 per-token
  if (unit === "/M" && n > 0 && n < 0.01) return `$${(n * 1_000_000).toFixed(n * 1_000_000 < 1 ? 3 : 2)}${unit}`;
  if (unit === "/秒" || unit === "/张") return `$${n}${unit}`;
  return `$${n}${unit}`;
}

export function formatContext(n?: number) {
  if (!n || n <= 0) return "—";
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(n % 1_000_000 === 0 ? 0 : 1)}M`;
  if (n >= 1_000) return `${Math.round(n / 1_000)}K`;
  return String(n);
}

export function capabilityLabels(caps?: Record<string, unknown>) {
  const supported = Array.isArray(caps?.supported_parameters)
    ? (caps?.supported_parameters as string[])
    : [];
  const map: Record<string, string> = {
    vision: "视觉",
    tools: "函数",
    tool_choice: "函数",
    reasoning: "推理",
    stream: "流式",
    response_format: "JSON",
    structured_outputs: "结构化",
    temperature: "温度",
  };
  const labels: string[] = [];
  for (const p of supported) {
    const label = map[p] || (p.length <= 8 ? p : "");
    if (label && !labels.includes(label)) labels.push(label);
  }
  return labels.slice(0, 7);
}

export function priceForModel(m: CatalogModel) {
  const kind = inferKind(m);
  if (kind === "video") {
    return { primary: formatMoney(m.sell_price?.media ?? m.sell_price?.output, "/秒"), secondary: "视频" };
  }
  if (kind === "image") {
    const img = m.sell_price?.image ?? m.sell_price?.output ?? m.sell_price?.input;
    return { primary: formatMoney(img, Number(img) > 0 && Number(img) < 0.01 ? "/M" : "/张"), secondary: "图像" };
  }
  return {
    primary: formatMoney(m.sell_price?.input, "/M"),
    secondary: formatMoney(m.sell_price?.output, "/M"),
  };
}

export const VENDOR_MARQUEE = [
  "OpenAI",
  "Anthropic",
  "Google",
  "DeepSeek",
  "Qwen",
  "Kimi",
  "Doubao",
  "GLM",
  "ByteDance",
  "xAI",
];

export const LEADERBOARD_DEMO = [
  { rank: "01", vendor: "Anthropic", name: "Claude Sonnet 4.6", share: "31.6%", delta: "-23.6 pp", href: "/models" },
  { rank: "02", vendor: "Anthropic", name: "Claude Opus 4.7", share: "15.9%", delta: "+7.3 pp", href: "/models" },
  { rank: "03", vendor: "Anthropic", name: "Claude Opus 4.6", share: "6.6%", delta: "-11.9 pp", href: "/models" },
  { rank: "04", vendor: "OpenAI", name: "GPT 5.5", share: "5.6%", delta: "+10.2 pp", href: "/models" },
  { rank: "05", vendor: "DeepSeek", name: "DeepSeek V4 Flash", share: "5.3%", delta: "+3.9 pp", href: "/models" },
];

/** 首页媒体墙：引用 ofox 公开 landing 资源，布局按 DESIGN.md 细线抬起，不做橙营销。 */
export const MEDIA_WALL = [
  { id: "volcengine/doubao-seedream-5.0-pro", name: "Doubao Seedream 5.0 Pro", kind: "image", src: "https://ofox.ai/landing-assets/gc-wall/doubao-seedream-5-0-pro-b.webp" },
  { id: "bytedance/seedance-2.5", name: "Seedance 2.5", kind: "video", src: "https://ofox.ai/landing-assets/gc-wall/seedance-2-5-c-v1.webp" },
  { id: "google/gemini-3-pro-image", name: "Gemini 3 Pro Image", kind: "image", src: "https://ofox.ai/landing-assets/gc-wall/gemini-3-pro-image-a.webp" },
  { id: "google/gemini-3.1-flash-image", name: "Gemini 3.1 Flash Image", kind: "image", src: "https://ofox.ai/landing-assets/gc-wall/gemini-3-1-flash-image-a.webp" },
  { id: "alibaba/wan-2.7", name: "Wan 2.7", kind: "video", src: "https://ofox.ai/landing-assets/gc-wall/wan-2-7-c-v1.webp" },
  { id: "openai/gpt-image-2", name: "GPT Image 2", kind: "image", src: "https://ofox.ai/landing-assets/gc-wall/gpt-image-2-a.webp" },
  { id: "bailian/qwen-image-3.0", name: "Qwen-Image 3.0", kind: "image", src: "https://ofox.ai/landing-assets/gc-wall/qwen-image-3-0-b.webp" },
  { id: "bytedance/seedance-2.0", name: "Seedance 2.0", kind: "video", src: "https://ofox.ai/landing-assets/gc-wall/seedance-2-5-c-v1.webp" },
];
