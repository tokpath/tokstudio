import type { CatalogModel } from "@/lib/catalog";
import { catalogModelUsable, exampleCurl, examplePath, modelEntry, protocolAllowsKeyVerify } from "@/lib/model-use";

export function apiHostFromEndpoint(endpoint: string): string {
  const raw = endpoint.trim();
  if (!raw) {
    return "";
  }
  try {
    const url = new URL(raw.includes("://") ? raw : `https://${raw}`);
    return url.host || "";
  } catch {
    return "";
  }
}

export function pickKeyExampleModel(allowlist: string[] | undefined, catalog: CatalogModel[], requested = ""): string {
  if (!requested) return "";
  return catalog.some(item=>item.id===requested&&catalogModelUsable(item)) && (!allowlist?.length || allowlist.includes(requested)) ? requested : "";
}

export function keyExampleFor(
  allowlist: string[] | undefined,
  catalog: CatalogModel[],
  endpoint: string,
  requested = "",
): { model: string; path: string; curl: string; verifiable: boolean } {
  const model = pickKeyExampleModel(allowlist, catalog, requested);
  const found = catalog.find((item) => item.id === model);
  const host = endpoint;
  if (!found) {
    return {
      model: "",
      path: "",
      curl: "",
      verifiable: false,
    };
  }
  const path = examplePath(found);
  return { model, path, curl: exampleCurl(model, path, host), verifiable: !!endpoint && protocolAllowsKeyVerify(path) };
}

export function keyVerifyRequest(model: string, path: string): { path: string; body: Record<string, unknown> } | null {
  const id = model.trim();
  if (!id || !protocolAllowsKeyVerify(path)) {
    return null;
  }
  if (path.startsWith("/v1/chat/completions")) {
    return { path: "/v1/chat/completions", body: { model: id, max_tokens:32, messages: [{ role: "user" as const, content: "ping" }] } };
  }
  if (path.startsWith("/v1/responses")) {
    return { path: "/v1/responses", body: { model: id, max_output_tokens:32, input: "ping" } };
  }
  if (path.startsWith("/v1/messages")) {
    return {
      path: "/v1/messages",
      body: { model: id, max_tokens: 32, messages: [{ role: "user" as const, content: "ping" }] },
    };
  }
  return null;
}
