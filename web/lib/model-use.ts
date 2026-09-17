import { apiBase } from "@/lib/api";
import type { CatalogModel } from "@/lib/catalog";
import { inferKind } from "@/lib/catalog";
import { loginHref, safeNextPath } from "@/lib/login-next";

export type ModelEntry = "chat" | "image" | "video" | "docs";

export function catalogModelUsable(model: Partial<CatalogModel> | null | undefined): boolean {
  const status = (model?.status || "available").trim().toLowerCase();
  return !["unavailable", "deprecated", "disabled", "rejected", "draft"].includes(status);
}

export function modelEntry(model: Partial<CatalogModel> | null | undefined): ModelEntry {
  const endpoints = endpointList(model?.capabilities);
  if (endpoints.some((item) => item.startsWith("/v1/images"))) {
    return "image";
  }
  if (endpoints.some((item) => item.startsWith("/v1/videos"))) {
    return "video";
  }
  if (endpoints.some((item) => item === "/v1/chat/completions" || item === "/v1/responses" || item === "/v1/messages")) {
    return "chat";
  }
  const kind = inferKind(model || {});
  if (kind === "image") {
    return "image";
  }
  if (kind === "video") {
    return "video";
  }
  if (kind === "text") {
    return "chat";
  }
  return "docs";
}

export function examplePath(model: Partial<CatalogModel> | null | undefined): string {
  const endpoints = endpointList(model?.capabilities);
  if (endpoints[0]) {
    return endpoints[0];
  }
  const entry = modelEntry(model);
  if (entry === "image") {
    return "/v1/images/generations";
  }
  if (entry === "video") {
    return "/v1/videos";
  }
  const kind = inferKind(model || {});
  if (kind === "embedding") {
    return "/v1/embeddings";
  }
  if (kind === "audio") {
    return "/v1/audio/transcriptions";
  }
  return "/v1/chat/completions";
}

/** Shell header that expands TOKENHUB_API_KEY. Single quotes would send the literal name. */
export const CURL_BEARER_HEADER = `-H "Authorization: Bearer \${TOKENHUB_API_KEY}"`;

export function exampleCurl(modelId: string, path: string, host = "localhost"): string {
  const id = modelId.trim() || "your-model";
  if (path.startsWith("/v1/audio")) {
    return `curl -sS https://${host}${path} ${CURL_BEARER_HEADER} -F file=@audio.mp3 -F model=${id}`;
  }
  const body = path.startsWith("/v1/embeddings")
    ? `{"model":"${id}","input":"hello"}`
    : path.startsWith("/v1/images")
      ? `{"model":"${id}","prompt":"a river"}`
      : path.startsWith("/v1/videos")
        ? `{"model":"${id}","prompt":"a river at dusk"}`
        : `{"model":"${id}","messages":[{"role":"user","content":"hi"}]}`;
  return `curl -sS https://${host}${path} ${CURL_BEARER_HEADER} -H "Content-Type: application/json" -d '${body}'`;
}

export function useModelHref(model: Pick<CatalogModel, "id"> & Partial<CatalogModel>, from?: string): string {
  const params = new URLSearchParams();
  const id = model.id.trim();
  if (id) {
    params.set("model", id);
  }
  const catalog = safeNextPath(from);
  if (catalog) {
    params.set("from", catalog);
  }
  const entry = modelEntry(model);
  if (entry === "image") {
    params.set("kind", "image");
    return `/app/media?${params.toString()}`;
  }
  if (entry === "video") {
    params.set("kind", "video");
    return `/app/media?${params.toString()}`;
  }
  if (entry === "docs") {
    return `/app/docs?${params.toString()}`;
  }
  const qs = params.toString();
  return qs ? `/app/playground?${qs}` : "/app/playground";
}

export async function resolveStartUsingHref(
  model: Pick<CatalogModel, "id"> & Partial<CatalogModel>,
  fetcher: typeof fetch = fetch,
): Promise<string> {
  const next = useModelHref(model);
  try {
    const meRes = await fetcher(`${apiBase}/v1/me`, { credentials: "include" });
    return meRes.ok ? next : loginHref(next);
  } catch {
    return loginHref(next);
  }
}

function endpointList(capabilities?: Record<string, unknown>): string[] {
  const raw = capabilities?.supported_endpoints;
  if (!Array.isArray(raw)) {
    return [];
  }
  return raw.map((item) => String(item));
}
