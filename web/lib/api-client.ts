import { z } from "zod";
import { APIErrorSchema } from "./contracts";
import { getClientLocale } from "./i18n";

export class APIError extends Error {
  constructor(public code: string, message: string, public requestId: string, public retryable: boolean, public nextAction?: string, public status?: number) { super(message); this.name = "APIError"; }
}

let csrfToken: string | undefined;
export function setCSRFToken(token?: string) { csrfToken = token; }
export function newActionKey() { return crypto.randomUUID(); }

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
  let response: Response;
  try { response = await fetch(path, { method, credentials: "include", cache: "no-store", headers, body: options.body !== undefined ? (multipart ? options.body as FormData : JSON.stringify(options.body)) : undefined, signal: options.signal }); }
  catch (error) {
    if (error instanceof Error && error.name === "AbortError") throw error;
    throw new APIError("network_unknown", "连接中断，操作结果尚未核实。请沿用本次操作重试或查询记录。", "", true);
  }
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
