export type ResourceFilter = { q: string; status: string; view: string };
export type ResourceListState = ResourceFilter & { cursor: string; history: string[] };

export function readResourceListState(): ResourceListState {
  let params = new URLSearchParams(typeof window === "undefined" ? "" : window.location.search);
  const returnTo = params.get("returnTo");
  if (typeof window !== "undefined" && params.has("record") && returnTo?.startsWith(window.location.pathname + "?")) params = new URLSearchParams(returnTo.slice(returnTo.indexOf("?") + 1));
  return { q: params.get("q") || "", status: params.get("status") || "", view: params.get("view") || "", cursor: params.get("cursor") || "", history: params.getAll("previous") };
}

export function updateResourceListURL(state: ResourceListState) {
  if (typeof window === "undefined" || !/^\/(manage|workspace)\//.test(window.location.pathname)) return;
  const url = new URL(window.location.href);
  for (const key of ["q", "status", "view", "cursor"] as const) {
    if (state[key]) url.searchParams.set(key, state[key]); else url.searchParams.delete(key);
  }
  url.searchParams.delete("previous");
  state.history.forEach(cursor => url.searchParams.append("previous", cursor));
  window.history.replaceState(window.history.state, "", url);
}

export function subjectURL(operatorID: string, resource: string, id: string, returnTo?: string): string {
  const params = new URLSearchParams({ record: id });
  if (returnTo && returnTo.startsWith(`/manage/${encodeURIComponent(operatorID)}/`) && !returnTo.startsWith("//")) params.set("returnTo", returnTo);
  return `/manage/${encodeURIComponent(operatorID)}/${resource}?${params}`;
}

export function subjectReturnURL(operatorID: string, resource: string): string {
  const fallback = `/manage/${encodeURIComponent(operatorID)}/${resource}`;
  if (typeof window === "undefined") return fallback;
  const target = new URLSearchParams(window.location.search).get("returnTo");
  if (target?.startsWith(`/manage/${encodeURIComponent(operatorID)}/`) && !target.includes("\\")) return target;
  const url = new URL(window.location.href);
  ["record", "returnTo"].forEach(key => url.searchParams.delete(key));
  return url.pathname.startsWith(fallback) ? `${url.pathname}${url.search}` : fallback;
}
