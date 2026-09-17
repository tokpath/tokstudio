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
});

describe("keyExampleFor", () => {
  it("keeps the env placeholder and a complete chat request", () => {
    const example = keyExampleFor(undefined, [gemini], "https://api.tokenhub.test/v1");
    expect(example.curl).toContain("$TOKENHUB_API_KEY");
    expect(example.curl).not.toMatch(/thk_/);
    expect(example.curl).toContain("/v1/chat/completions");
    expect(example.curl).toContain('"model":"google/gemini-flash"');
    expect(example.curl).toContain("Content-Type: application/json");
  });

  it("uses embeddings path when that is what the key allows", () => {
    const example = keyExampleFor(["openai/text-embedding-3"], [embed, gemini], "https://api.tokenhub.test/v1");
    expect(example.model).toBe("openai/text-embedding-3");
    expect(example.path).toBe("/v1/embeddings");
    expect(example.curl).toContain("/v1/embeddings");
    expect(example.curl).toContain("$TOKENHUB_API_KEY");
  });
});

describe("keyVerifyRequest", () => {
  it("posts chat completions for text models", () => {
    expect(keyVerifyRequest("google/gemini-flash", "/v1/chat/completions")).toEqual({
      path: "/v1/chat/completions",
      body: { model: "google/gemini-flash", messages: [{ role: "user", content: "ping" }] },
    });
  });
});

describe("apiHostFromEndpoint", () => {
  it("reads the host from an https /v1 endpoint", () => {
    expect(apiHostFromEndpoint("https://api.tokenhub.test/v1")).toBe("api.tokenhub.test");
  });
});
