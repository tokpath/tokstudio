import { apiBase } from "@/lib/api";
import { generatedFetch, generatedOperations, type HttpMethod } from "@/lib/generated/api";

export function hasGeneratedPath(method: HttpMethod, path: string): boolean {
  return generatedOperations.some((item) => item.method === method && item.path === path);
}

export async function apiClient<T>(method: HttpMethod, path: string, init?: RequestInit): Promise<T> {
  return generatedFetch<T>(apiBase, method, path, { credentials: "include", ...init });
}
