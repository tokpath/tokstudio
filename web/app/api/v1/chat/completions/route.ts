export const dynamic = "force-dynamic";
export const runtime = "nodejs";

const hopByHop = new Set(["connection", "keep-alive", "transfer-encoding", "upgrade", "host", "content-length"]);

function apiOrigin(): string {
  return (process.env.TOKENHUB_API_INTERNAL_URL || "http://127.0.0.1:8080").replace(/\/$/, "");
}

function forwardHeaders(source: Headers): Headers {
  const headers = new Headers();
  source.forEach((value, key) => {
    if (!hopByHop.has(key.toLowerCase())) {
      headers.set(key, value);
    }
  });
  return headers;
}

/** 浏览器的 /api/v1/chat/completions 走这里，把上游 SSE 原样转出去，不在 Next 里攒完整响应。 */
export async function POST(request: Request) {
  const upstream = await fetch(`${apiOrigin()}/v1/chat/completions`, {
    method: "POST",
    headers: forwardHeaders(request.headers),
    body: request.body,
    duplex: "half",
  } as RequestInit & { duplex: "half" });
  return new Response(upstream.body, {
    status: upstream.status,
    headers: forwardHeaders(upstream.headers),
  });
}
