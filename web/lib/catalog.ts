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

export type CatalogQuery = {
  vendor?: string;
  kind?: string;
  q?: string;
  id?: string;
  limit?: number;
};

export type FacetCount = { id: string; count: number };

export type CatalogFacets = {
  kinds: FacetCount[];
  vendors: FacetCount[];
};

export type CatalogPage = {
  items: CatalogModel[];
  total: number;
  facets: CatalogFacets;
};

const EMPTY_PAGE: CatalogPage = { items: [], total: 0, facets: { kinds: [], vendors: [] } };

export type AdminModel = {
  id: string;
  vendor: string;
  display_name: string;
  status: string;
  sync_state?: string;
  created_by_user_id?: string;
  reviewed_by_user_id?: string;
  capabilities?: Record<string, unknown>;
  sell_price?: Record<string, unknown>;
  providers?: string[];
};

export function publicModelsPath(query: CatalogQuery = {}): string {
  const params = catalogSearchParams(query, { includeId: true, includeLimit: true });
  const qs = params.toString();
  return qs ? `/v1/public/models?${qs}` : "/v1/public/models";
}

export function catalogHref(basePath: string, query: CatalogQuery = {}): string {
  const qs = catalogSearchParams(query).toString();
  return qs ? `${basePath}?${qs}` : basePath;
}

export function parseCatalogSearchParams(query: {
  kind?: string;
  output?: string;
  vendor?: string;
  q?: string;
}): CatalogQuery {
  const kind = (query.kind || query.output || "").trim();
  return {
    vendor: query.vendor?.trim() || undefined,
    kind: kind && kind !== "all" ? kind : undefined,
    q: query.q?.trim() || undefined,
  };
}

function catalogSearchParams(
  query: CatalogQuery,
  opts: { includeId?: boolean; includeLimit?: boolean } = {},
): URLSearchParams {
  const params = new URLSearchParams();
  if (query.vendor?.trim()) {
    params.set("vendor", query.vendor.trim());
  }
  const kind = query.kind?.trim();
  if (kind && kind !== "all") {
    params.set("kind", kind);
  }
  if (query.q?.trim()) {
    params.set("q", query.q.trim());
  }
  if (opts.includeId && query.id?.trim()) {
    params.set("id", query.id.trim());
  }
  if (opts.includeLimit && query.limit && query.limit > 0) {
    params.set("limit", String(query.limit));
  }
  return params;
}

function normalizeCatalogModel(m: CatalogModel): CatalogModel {
  return {
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
  };
}

/** 只读后端公开目录。失败或空列表就空着，不再用本地 ofox 快照顶上。 */
export async function loadCatalogPage(host: string, query: CatalogQuery = {}): Promise<CatalogPage> {
  try {
    const data = await fetchAPI<{ items?: CatalogModel[]; total?: number; facets?: CatalogFacets }>(
      publicModelsPath(query),
      { host },
    );
    const items = Array.isArray(data.items) ? data.items.map(normalizeCatalogModel) : [];
    return {
      items,
      total: typeof data.total === "number" ? data.total : items.length,
      facets: {
        kinds: Array.isArray(data.facets?.kinds) ? data.facets.kinds : [],
        vendors: Array.isArray(data.facets?.vendors) ? data.facets.vendors : [],
      },
    };
  } catch {
    return EMPTY_PAGE;
  }
}

export async function loadCatalog(host: string, query: CatalogQuery = {}): Promise<CatalogModel[]> {
  return (await loadCatalogPage(host, query)).items;
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

export type PriceUnits = { perSec?: string; perImage?: string };

export function formatMoney(raw: unknown, unit = "/M") {
  if (raw == null || raw === "") return "—";
  const n = Number(raw);
  if (!Number.isFinite(n)) return String(raw);
  if (n === 0) return "$0";
  // ofox / OpenRouter 风格：prompt 常为 per-token
  if (unit === "/M" && n > 0 && n < 0.01) return `$${(n * 1_000_000).toFixed(n * 1_000_000 < 1 ? 3 : 2)}${unit}`;
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
    vision: "vision",
    tools: "tools",
    tool_choice: "tools",
    reasoning: "reasoning",
    stream: "stream",
    response_format: "json",
    structured_outputs: "structured",
    temperature: "temperature",
  };
  const labels: string[] = [];
  for (const p of supported) {
    const label = map[p] || (p.length <= 8 ? p : "");
    if (label && !labels.includes(label)) labels.push(label);
  }
  return labels.slice(0, 7);
}

export function priceForModel(m: CatalogModel, units: PriceUnits = {}) {
  const kind = inferKind(m);
  const perSec = units.perSec ?? "/s";
  const perImage = units.perImage ?? "/img";
  if (kind === "video") {
    return { primary: formatMoney(m.sell_price?.media ?? m.sell_price?.output, perSec), secondary: kind, kind };
  }
  if (kind === "image") {
    const img = m.sell_price?.image ?? m.sell_price?.output ?? m.sell_price?.input;
    return { primary: formatMoney(img, Number(img) > 0 && Number(img) < 0.01 ? "/M" : perImage), secondary: kind, kind };
  }
  return {
    primary: formatMoney(m.sell_price?.input, "/M"),
    secondary: formatMoney(m.sell_price?.output, "/M"),
    kind,
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

export function modelEditHref(publicId: string): string {
  return `/admin/models/${publicId}`;
}

export function formatSellPrice(price?: Record<string, unknown> | null): string {
  if (!price) {
    return "—";
  }
  const parts: string[] = [];
  if (price.input != null && String(price.input) !== "") {
    parts.push(`in ${price.input}`);
  }
  if (price.output != null && String(price.output) !== "") {
    parts.push(`out ${price.output}`);
  }
  if (price.video_second != null && String(price.video_second) !== "") {
    parts.push(`video ${price.video_second}`);
  }
  if (price.image_count != null && String(price.image_count) !== "") {
    parts.push(`image ${price.image_count}`);
  }
  if (price.audio_second != null && String(price.audio_second) !== "") {
    parts.push(`audio ${price.audio_second}`);
  }
  return parts.length > 0 ? parts.join(" / ") : "—";
}

export function supportedParametersText(capabilities?: Record<string, unknown> | null): string {
  const raw = capabilities?.supported_parameters;
  if (Array.isArray(raw)) {
    return raw.map(String).join(", ");
  }
  if (typeof raw === "string") {
    return raw;
  }
  return "";
}

export function parseSupportedParameters(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

export function extraCapabilitiesJSON(capabilities?: Record<string, unknown> | null): string {
  if (!capabilities) {
    return "";
  }
  const extra = { ...capabilities };
  delete extra.supported_parameters;
  return Object.keys(extra).length > 0 ? JSON.stringify(extra, null, 2) : "";
}

export function buildCapabilities(params: string, extraJSON: string): Record<string, unknown> {
  const extra = extraJSON.trim() ? (JSON.parse(extraJSON) as Record<string, unknown>) : {};
  return { ...extra, supported_parameters: parseSupportedParameters(params) };
}
