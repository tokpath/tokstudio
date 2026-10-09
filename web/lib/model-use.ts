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
  return "";
}

/** Shell header that expands TOKENHUB_API_KEY. Single quotes would send the literal name. */
export const CURL_BEARER_HEADER = `-H "Authorization: Bearer \${TOKENHUB_API_KEY}"`;

const VERIFY_PATHS = ["/v1/chat/completions", "/v1/responses", "/v1/messages"] as const;

export function exampleJSONBody(modelId: string, path: string): Record<string, unknown> | null {
  const id = modelId.trim() || "your-model";
  if (path.startsWith("/v1/chat/completions")) {
    return { model: id, max_tokens: 32, messages: [{ role: "user", content: "hi" }] };
  }
  if (path.startsWith("/v1/responses")) {
    return { model: id, max_output_tokens: 32, input: "hi" };
  }
  if (path.startsWith("/v1/messages")) {
    return { model: id, max_tokens: 32, messages: [{ role: "user", content: "hi" }] };
  }
  if (path.startsWith("/v1/embeddings")) {
    return { model: id, input: "hello" };
  }
  if (path.startsWith("/v1/images")) {
    return { model: id, prompt: "a river" };
  }
  if (path.startsWith("/v1/videos")) {
    return { model: id, prompt: "a river at dusk", duration: 5, resolution: "720p" };
  }
  return null;
}

export function protocolAllowsKeyVerify(path: string): boolean {
  return VERIFY_PATHS.some((item) => path === item || false);
}

export function exampleCurl(modelId: string, path: string, host = ""): string {
  if (!host || !path || !modelId.trim()) return "";
  const base = (host.includes("://") ? host : `https://${host}`).replace(/\/$/, "").replace(/\/v1$/, "");
  const id = modelId.trim();
  if (path.startsWith("/v1/audio")) {
    return `curl -sS ${base}${path} ${CURL_BEARER_HEADER} -F file=@audio.mp3 -F model=${id}`;
  }
  const body = exampleJSONBody(id, path);
  if (!body) {
    return `curl -sS ${base}${path} ${CURL_BEARER_HEADER}`;
  }
  return `curl -sS ${base}${path} ${CURL_BEARER_HEADER} -H "Content-Type: application/json" -d '${JSON.stringify(body).replaceAll("'", "'\"'\"'")}'`;
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
  params.set("tab",modelEntry(model)==="chat"?"agent":"protocol");
  return `/app/docs?${params}`;
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
