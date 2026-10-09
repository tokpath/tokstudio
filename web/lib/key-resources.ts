import { errorMessageFromBody, readResponseBody } from "./submit-result";
import type { ListLoadResult } from "./list-resource";

/** Follow the server's cursor contract; never silently omit the 101st Key/model. */
export async function fetchKeyPages<T>(url: string, signal?: AbortSignal): Promise<ListLoadResult<T>> {
  const items: T[] = [];
  const cursors = new Set<string>();
  let cursor = "";
  try {
    do {
      const next = new URL(url, typeof window === "undefined" ? "http://localhost" : window.location.origin);
      next.searchParams.set("limit", "100");
      if (cursor) next.searchParams.set("cursor", cursor);
      const response = await fetch(url.startsWith("http") ? next.toString() : next.pathname + next.search, {credentials: "include", signal});
      const body = await readResponseBody(response) as {items?: T[]; next_cursor?: string; error?: {code?: string}};
      if (!response.ok) return {ok: false, status: response.status, code: body.error?.code, message: errorMessageFromBody(body, "")};
      items.push(...(body.items ?? []));
      cursor = body.next_cursor ?? "";
      if (cursor && cursors.has(cursor)) return {ok: false, message: "Invalid pagination cursor"};
      cursors.add(cursor);
    } while (cursor);
    return {ok: true, items};
  } catch {return {ok: false, network: true};}
}
