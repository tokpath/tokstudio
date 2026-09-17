import { describe, expect, it } from "vitest";
import { apiHostFromEndpoint, keyExampleFor, keyVerifyRequest, pickKeyExampleModel } from "./key-example";

const echo = { id: "tokenhub/echo-1", kind: "text" as const, status: "available" };
const gemini = { id: "google/gemini-flash", kind: "text" as const, status: "available" };
const embed = {
  id: "openai/text-embedding-3",
  kind: "embedding" as const,
  status: "available",
  capabilities: { supported_endpoints: ["/v1/embeddings"] },
};

describe("pickKeyExampleModel", () => {
  it("stays inside a non-empty allowlist", () => {
    expect(pickKeyExampleModel(["google/gemini-flash"], [echo, gemini])).toBe("google/gemini-flash");
  });

  it("prefers a chat model when the allowlist is empty", () => {
    expect(pickKeyExampleModel([], [embed, gemini])).toBe("google/gemini-flash");
  });

  it("does not invent a default model when the catalog is empty", () => {
    expect(pickKeyExampleModel([], [])).toBe("");
    expect(pickKeyExampleModel(["google/gemini-flash"], [])).toBe("");
  });
});

describe("keyExampleFor", () => {
  it("keeps the env placeholder and a complete chat request", () => {
    const example = keyExampleFor(undefined, [gemini], "https://api.tokenhub.test/v1");
    expect(example.curl).toContain(`-H "Authorization: Bearer \${TOKENHUB_API_KEY}"`);
    expect(example.curl).not.toContain("'Authorization:");
    expect(example.curl).not.toMatch(/thk_/);
    expect(example.curl).toContain("/v1/chat/completions");
    expect(example.curl).toContain('"model":"google/gemini-flash"');
    expect(example.curl).toContain("Content-Type: application/json");
  });

  it("uses embeddings path when that is what the key allows", () => {
    const example = keyExampleFor(["openai/text-embedding-3"], [embed, gemini], "https://api.tokenhub.test/v1");
    expect(example.model).toBe("openai/text-embedding-3");
    expect(example.path).toBe("/v1/embeddings");
    expect(example.verifiable).toBe(false);
    expect(example.curl).toContain("/v1/embeddings");
    expect(example.curl).toContain('"input":"hello"');
    expect(example.curl).not.toContain("/v1/chat/completions");
    expect(example.curl).toContain(`-H "Authorization: Bearer \${TOKENHUB_API_KEY}"`);
    expect(example.curl).not.toContain("'Authorization:");
  });

  it("builds Anthropic and Responses samples without rewriting them as chat", () => {
    const messages = keyExampleFor(
      ["anthropic/claude"],
      [{ id: "anthropic/claude", kind: "text", status: "available", capabilities: { supported_endpoints: ["/v1/messages"] } }],
      "https://api.tokenhub.test/v1",
    );
    expect(messages.path).toBe("/v1/messages");
    expect(messages.verifiable).toBe(true);
    expect(messages.curl).toContain("/v1/messages");
    expect(messages.curl).toContain('"max_tokens":32');
    expect(messages.curl).not.toContain("/v1/chat/completions");

    const responses = keyExampleFor(
      ["openai/gpt"],
      [{ id: "openai/gpt", kind: "text", status: "available", capabilities: { supported_endpoints: ["/v1/responses"] } }],
      "https://api.tokenhub.test/v1",
    );
    expect(responses.path).toBe("/v1/responses");
    expect(responses.curl).toContain("/v1/responses");
    expect(responses.curl).toContain('"input":"hi"');
    expect(responses.curl).not.toContain('"messages"');
  });

  it("keeps a chat sample but does not mark it verifiable without a catalog model", () => {
    const example = keyExampleFor(undefined, [], "https://api.tokenhub.test/v1");
    expect(example.model).toBe("");
    expect(example.verifiable).toBe(false);
    expect(example.curl).toContain("/v1/chat/completions");
    expect(example.curl).toContain('"model":"your-model"');
    expect(example.curl).not.toContain("tokenhub/echo-1");
  });
});

describe("keyVerifyRequest", () => {
  it("posts chat completions for text models", () => {
    expect(keyVerifyRequest("google/gemini-flash", "/v1/chat/completions")).toEqual({
      path: "/v1/chat/completions",
      body: { model: "google/gemini-flash", messages: [{ role: "user", content: "ping" }] },
    });
  });

  it("posts Responses and Messages on their own paths", () => {
    expect(keyVerifyRequest("openai/gpt", "/v1/responses")).toEqual({
      path: "/v1/responses",
      body: { model: "openai/gpt", input: "ping" },
    });
    expect(keyVerifyRequest("anthropic/claude", "/v1/messages")).toEqual({
      path: "/v1/messages",
      body: { model: "anthropic/claude", max_tokens: 32, messages: [{ role: "user", content: "ping" }] },
    });
  });

  it("does not rewrite unverifiable protocols into chat completions", () => {
    expect(keyVerifyRequest("openai/text-embedding-3", "/v1/embeddings")).toBeNull();
    expect(keyVerifyRequest("bytedance/seedance", "/v1/videos")).toBeNull();
    expect(keyVerifyRequest("whisper", "/v1/audio/transcriptions")).toBeNull();
    expect(keyVerifyRequest("google/gemini-flash", "/v1/chat/completions")).not.toBeNull();
    expect(keyVerifyRequest("", "/v1/chat/completions")).toBeNull();
  });
});

describe("apiHostFromEndpoint", () => {
  it("reads the host from an https /v1 endpoint", () => {
    expect(apiHostFromEndpoint("https://api.tokenhub.test/v1")).toBe("api.tokenhub.test");
  });
});
