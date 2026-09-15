export type ListPhase = "loading" | "empty" | "ready" | "error" | "unauthorized" | "stale";

export type ListLoadResult<T> = {
  ok: boolean;
  status?: number;
  items?: T[];
  message?: string;
  code?: string;
  network?: boolean;
};

export type ListSnapshot<T> = {
  phase: ListPhase;
  items: T[];
  message: string;
  httpStatus?: number;
  code?: string;
};

export function initialListSnapshot<T>(): ListSnapshot<T> {
  return { phase: "loading", items: [], message: "" };
}

export function isSessionLoss(status?: number, code?: string, message?: string): boolean {
  if (status === 401) {
    return true;
  }
  if (status !== 403) {
    return false;
  }
  if (code === "authentication_error") {
    return true;
  }
  return message === "未登录" || message === "未授权";
}

function errorFromBody(body: unknown): { message?: string; code?: string } {
  if (!body || typeof body !== "object") {
    return {};
  }
  const error = (body as { error?: { message?: string; code?: string } }).error;
  if (!error || typeof error !== "object") {
    return {};
  }
  return { message: error.message, code: error.code };
}

export function applyListResult<T>(previous: ListSnapshot<T>, result: ListLoadResult<T>): ListSnapshot<T> {
  if (result.ok) {
    const items = result.items ?? [];
    return {
      phase: items.length === 0 ? "empty" : "ready",
      items,
      message: result.message ?? "",
    };
  }
  const status = result.status;
  const message = result.message ?? "";
  if (previous.items.length > 0) {
    return {
      phase: "stale",
      items: previous.items,
      message,
      httpStatus: status,
      code: result.code,
    };
  }
  if (isSessionLoss(status, result.code, message)) {
    return { phase: "unauthorized", items: [], message, httpStatus: status, code: result.code };
  }
  if (status === 403) {
    return { phase: "unauthorized", items: [], message, httpStatus: status, code: result.code };
  }
  return { phase: "error", items: [], message, httpStatus: status, code: result.code };
}

export async function fetchListItems<T>(url: string): Promise<ListLoadResult<T>> {
  try {
    const response = await fetch(url, { credentials: "include" });
    const body: unknown = await response.json().catch(() => ({}));
    const record = body && typeof body === "object" ? (body as { items?: T[] }) : {};
    const items = Array.isArray(record.items) ? record.items : [];
    const error = errorFromBody(body);
    return {
      ok: response.ok,
      status: response.status,
      items,
      message: error.message,
      code: error.code,
    };
  } catch {
    return { ok: false, network: true, items: [] };
  }
}
