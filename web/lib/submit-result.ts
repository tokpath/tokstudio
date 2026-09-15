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
