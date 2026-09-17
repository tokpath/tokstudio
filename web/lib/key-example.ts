import type { CatalogModel } from "@/lib/catalog";
import { catalogModelUsable, exampleCurl, examplePath, modelEntry } from "@/lib/model-use";

const FALLBACK_MODEL = "tokenhub/echo-1";

export function apiHostFromEndpoint(endpoint: string): string {
  const raw = endpoint.trim();
  if (!raw) {
    return "localhost";
  }
  try {
    const url = new URL(raw.includes("://") ? raw : `https://${raw}`);
    return url.host || "localhost";
  } catch {
    return "localhost";
  }
}

export function pickKeyExampleModel(allowlist: string[] | undefined, catalog: CatalogModel[]): string {
  const usable = catalog.filter((item) => catalogModelUsable(item));
  if (allowlist && allowlist.length > 0) {
    const allowedUsable = allowlist.filter((id) => usable.some((item) => item.id === id));
    const chat = allowedUsable.find((id) => {
      const found = usable.find((item) => item.id === id);
      return found ? modelEntry(found) === "chat" : false;
    });
    return chat || allowedUsable[0] || allowlist[0];
  }
  const chat = usable.find((item) => modelEntry(item) === "chat");
  return chat?.id || usable[0]?.id || FALLBACK_MODEL;
}

export function keyExampleFor(
  allowlist: string[] | undefined,
  catalog: CatalogModel[],
  endpoint: string,
): { model: string; path: string; curl: string } {
  const model = pickKeyExampleModel(allowlist, catalog);
  const found = catalog.find((item) => item.id === model);
  const path = examplePath(found || { id: model, kind: "text" });
  return { model, path, curl: exampleCurl(model, path, apiHostFromEndpoint(endpoint)) };
}

export function keyVerifyRequest(model: string, path: string): { path: string; body: Record<string, unknown> } {
  if (path.startsWith("/v1/embeddings")) {
    return { path: "/v1/embeddings", body: { model, input: "ping" } };
  }
  if (path.startsWith("/v1/images")) {
    return { path: "/v1/images/generations", body: { model, prompt: "ping" } };
  }
  if (path.startsWith("/v1/videos")) {
    return { path: "/v1/videos", body: { model, prompt: "ping" } };
  }
  return {
    path: "/v1/chat/completions",
    body: { model, messages: [{ role: "user" as const, content: "ping" }] },
  };
}
