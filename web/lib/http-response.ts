export class HttpResponseError extends Error {
  readonly status: number;
  readonly code?: string;
  readonly body: unknown;

  constructor(status: number, body: unknown) {
    const envelope = body && typeof body === "object" ? body as Record<string, unknown> : {};
    const detail = envelope.error && typeof envelope.error === "object"
      ? envelope.error as Record<string, unknown> : envelope;
    super(typeof detail.message === "string" && detail.message ? detail.message : `HTTP ${status}`);
    this.name = "HttpResponseError";
    this.status = status;
    this.code = typeof detail.code === "string" ? detail.code : undefined;
    this.body = body;
  }
}

export async function readJsonResponse<T>(response: Response): Promise<T> {
  let body: unknown;
  try {
    body = await response.json();
  } catch (error) {
    if (response.ok) throw error;
    throw new HttpResponseError(response.status, undefined);
  }
  if (!response.ok) throw new HttpResponseError(response.status, body);
  return body as T;
}
