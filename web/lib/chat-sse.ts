/** 从还没读完的 SSE 缓冲里取出已经结束的 data 事件，剩下半截留给下一次读取。 */
export function takeSSEEvents(buffer: string): { events: string[]; rest: string } {
  const parts = buffer.split(/\n\n/);
  const rest = parts.pop() ?? "";
  const events: string[] = [];
  for (const part of parts) {
    for (const line of part.split(/\n/)) {
      const trimmed = line.replace(/\r$/, "");
      if (trimmed.startsWith("data:")) {
        events.push(trimmed.slice(5).trimStart());
      }
    }
  }
  return { events, rest };
}

/** 取出这一条 chat chunk 新增的文本。没有文本时返回空字符串。 */
export function deltaText(event: string): string {
  if (!event || event === "[DONE]") {
    return "";
  }
  try {
    const body = JSON.parse(event) as { choices?: { delta?: { content?: unknown } }[] };
    const content = body.choices?.[0]?.delta?.content;
    return typeof content === "string" ? content : "";
  } catch {
    return "";
  }
}

/** 流里如果直接带了错误对象，把消息取出来。 */
export function streamError(event: string): string | null {
  if (!event || event === "[DONE]") {
    return null;
  }
  try {
    const body = JSON.parse(event) as { error?: { message?: unknown } };
    return typeof body.error?.message === "string" ? body.error.message : null;
  } catch {
    return null;
  }
}
