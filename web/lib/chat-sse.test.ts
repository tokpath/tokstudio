import { describe, expect, it } from "vitest";
import { deltaText, streamError, takeSSEEvents } from "./chat-sse";

describe("chat SSE", () => {
  it("returns a partial frame until the blank line arrives", () => {
    const first = takeSSEEvents('data: {"choices":[{"delta":{"content":"He"}}]}');
    expect(first.events).toEqual([]);
    expect(first.rest).toContain("He");
    const done = takeSSEEvents(`${first.rest}\n\ndata: [DONE]\n\n`);
    expect(done.events).toEqual(['{"choices":[{"delta":{"content":"He"}}]}', "[DONE]"]);
    expect(deltaText(done.events[0])).toBe("He");
    expect(deltaText(done.events[1])).toBe("");
  });

  it("reads an error event without treating it as text", () => {
    expect(streamError('{"error":{"message":"上游中断"}}')).toBe("上游中断");
    expect(deltaText('{"error":{"message":"上游中断"}}')).toBe("");
  });
});
