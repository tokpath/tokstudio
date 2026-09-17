import type { CatalogModel } from "@/lib/catalog";
import { catalogModelUsable, exampleCurl, examplePath, modelEntry, protocolAllowsKeyVerify } from "@/lib/model-use";

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
    return chat || allowedUsable[0] || "";
  }
  const chat = usable.find((item) => modelEntry(item) === "chat");
  return chat?.id || usable[0]?.id || "";
}

export function keyExampleFor(
  allowlist: string[] | undefined,
  catalog: CatalogModel[],
  endpoint: string,
): { model: string; path: string; curl: string; verifiable: boolean } {
  const model = pickKeyExampleModel(allowlist, catalog);
  const found = catalog.find((item) => item.id === model);
  const host = apiHostFromEndpoint(endpoint);
  if (!found) {
    return {
      model: "",
      path: "",
      curl: exampleCurl("your-model", "/v1/chat/completions", host),
      verifiable: false,
    };
  }
  const path = examplePath(found);
  return { model, path, curl: exampleCurl(model, path, host), verifiable: protocolAllowsKeyVerify(path) };
}

export function keyVerifyRequest(model: string, path: string): { path: string; body: Record<string, unknown> } | null {
  const id = model.trim();
  if (!id || !protocolAllowsKeyVerify(path)) {
    return null;
  }
  if (path.startsWith("/v1/chat/completions")) {
    return { path: "/v1/chat/completions", body: { model: id, messages: [{ role: "user" as const, content: "ping" }] } };
  }
  if (path.startsWith("/v1/responses")) {
    return { path: "/v1/responses", body: { model: id, input: "ping" } };
  }
  if (path.startsWith("/v1/messages")) {
    return {
      path: "/v1/messages",
      body: { model: id, max_tokens: 32, messages: [{ role: "user" as const, content: "ping" }] },
    };
  }
  return null;
}
