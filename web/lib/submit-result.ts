export function errorMessageFromBody(body: unknown, fallback: string): string {
  if (!body || typeof body !== "object") {
    return fallback;
  }
  const error = (body as { error?: { message?: string } }).error;
  const message = error && typeof error === "object" ? error.message : undefined;
  if (typeof message === "string" && message.trim()) {
    return message.trim();
  }
  return fallback;
}

export async function readResponseBody(response: Response): Promise<unknown> {
  return response.json().catch(() => ({}));
}

/** Explicit confirm outcome: only `true` means the request succeeded. */
export async function confirmJsonAction(options: {
  request: () => Promise<Response>;
  onSuccess?: (body: unknown, response: Response) => void | Promise<void>;
  onError: (message: string) => void;
  failFallback: string;
  networkMessage: string;
}): Promise<boolean> {
  try {
    const response = await options.request();
    const body = await readResponseBody(response);
    if (!response.ok) {
      options.onError(errorMessageFromBody(body, options.failFallback));
      return false;
    }
    await options.onSuccess?.(body, response);
    return true;
  } catch {
    options.onError(options.networkMessage);
    return false;
  }
}

export async function copyText(text: string): Promise<boolean> {
  try {
    if (typeof navigator === "undefined" || !navigator.clipboard?.writeText) {
      return false;
    }
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}
