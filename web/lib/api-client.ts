import { z } from "zod";
import { APIErrorSchema } from "./contracts";
import { getClientLocale } from "./i18n";

export class APIError extends Error {
  constructor(public code: string, message: string, public requestId: string, public retryable: boolean, public nextAction?: string, public status?: number) { super(message); this.name = "APIError"; }
}

let csrfToken: string | undefined;
export function setCSRFToken(token?: string) { csrfToken = token; }
export function newActionKey() { return crypto.randomUUID(); }

const requestTimeoutMs = 12_000;

// An idle tab often keeps a connection the proxy has already closed. The next
// click then waits until the browser gives up. A short timeout drops that
// attempt; a read is tried once more on a new connection.
export function warmConnection() {
  const ping = () => {
    const signal = typeof AbortSignal.timeout === "function" ? AbortSignal.timeout(8_000) : undefined;
    return fetch("/api/healthz", { cache: "no-store", credentials: "same-origin", signal }).catch(() => undefined);
  };
  void ping().then(() => ping());
}

async function readResponse<T extends z.ZodTypeAny>(response: Response, schema: T): Promise<z.infer<T>> {
  let body: unknown;
  try { body = await response.json(); }
  catch { throw new APIError("invalid_response", "服务返回了无法识别的结果，请稍后查询操作状态。", response.headers.get("X-Request-ID") || "", true, undefined, response.status); }
  if (!response.ok) {
    const parsed = APIErrorSchema.safeParse(body);
    if (parsed.success) { const e = parsed.data.error; throw new APIError(e.code, e.message, e.request_id, e.retryable, e.next_action, response.status); }
    throw new APIError("service_error", response.status === 401 ? "请先登录。" : "服务暂时无法完成操作，请稍后重试。", response.headers.get("X-Request-ID") || "", response.status >= 500, undefined, response.status);
  }
  const result = schema.safeParse(body);
  if (!result.success) throw new APIError("contract_mismatch", "服务数据格式与当前页面不一致，请联系经营方核实。", response.headers.get("X-Request-ID") || "", false);
  return result.data;
}

export async function request<T extends z.ZodTypeAny>(path: string, schema: T, options: { method?: "GET" | "POST" | "PATCH" | "DELETE"; body?: unknown; actionKey?: string; signal?: AbortSignal } = {}): Promise<z.infer<T>> {
  const method = options.method || "GET";
  if (!path.startsWith("/api/v1/")) throw new Error("Business API path must start with /api/v1/");
  const headers = new Headers({ Accept: "application/json", "Accept-Language": getClientLocale() });
  const multipart = options.body instanceof FormData;
  if (options.body !== undefined && !multipart) headers.set("Content-Type", "application/json");
  if (method !== "GET") {
    if (!options.actionKey) throw new Error("Writing a business action requires its stable idempotency key");
    headers.set("Idempotency-Key", options.actionKey);
    if (csrfToken) headers.set("X-CSRF-Token", csrfToken);
  }
  const attempts = method === "GET" ? 2 : 1;
  let lastError: unknown;
  for (let attempt = 0; attempt < attempts; attempt++) {
    const controller = new AbortController();
    let timedOut = false;
    const timer = setTimeout(() => { timedOut = true; controller.abort(); }, requestTimeoutMs);
    const stop = () => controller.abort();
    options.signal?.addEventListener("abort", stop);
    try {
      if (options.signal?.aborted) throw new DOMException("The operation was aborted.", "AbortError");
      const response = await fetch(path, { method, credentials: "include", cache: "no-store", headers, body: options.body !== undefined ? (multipart ? options.body as FormData : JSON.stringify(options.body)) : undefined, signal: controller.signal });
      return await readResponse(response, schema);
    } catch (error) {
      if (options.signal?.aborted) throw error;
      lastError = timedOut || !(error instanceof APIError) ? new APIError("network_unknown", "连接中断，操作结果尚未核实。请沿用本次操作重试或查询记录。", "", true) : error;
      if (error instanceof APIError) throw error;
    } finally {
      clearTimeout(timer);
      options.signal?.removeEventListener("abort", stop);
    }
  }
  throw lastError;
}
