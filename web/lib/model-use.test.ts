import { describe, expect, it } from "vitest";
import { loginHref } from "./login-next";
import { exampleCurl, examplePath, modelEntry, useModelHref } from "./model-use";

describe("modelEntry", () => {
  it("prefers supported endpoints over kind", () => {
    expect(modelEntry({ kind: "text", capabilities: { supported_endpoints: ["/v1/images/generations"] } })).toBe("image");
    expect(modelEntry({ kind: "image", capabilities: { supported_endpoints: ["/v1/chat/completions"] } })).toBe("chat");
    expect(modelEntry({ kind: "embedding" })).toBe("docs");
  });
});

describe("useModelHref", () => {
  it("routes chat image video and docs separately", () => {
    expect(useModelHref({ id: "tokenhub/echo-1", kind: "text" })).toBe("/app/playground?model=tokenhub%2Fecho-1");
    expect(useModelHref({ id: "bytedance/seedream", kind: "image" })).toBe(
      "/app/media?model=bytedance%2Fseedream&kind=image",
    );
    expect(useModelHref({ id: "bytedance/seedance", kind: "video" })).toBe(
      "/app/media?model=bytedance%2Fseedance&kind=video",
    );
    expect(useModelHref({ id: "openai/text-embedding-3", kind: "embedding" })).toBe(
      "/app/docs?model=openai%2Ftext-embedding-3",
    );
  });
});

describe("exampleCurl", () => {
  it("points embeddings at /v1/embeddings instead of chat", () => {
    const path = examplePath({ kind: "embedding", id: "openai/text-embedding-3" });
    expect(path).toBe("/v1/embeddings");
    expect(exampleCurl("openai/text-embedding-3", path, "api.test")).toContain("/v1/embeddings");
    expect(exampleCurl("openai/text-embedding-3", path, "api.test")).toContain('"model":"openai/text-embedding-3"');
  });
});

describe("guest start href", () => {
  it("keeps login next on the matching entry", () => {
    expect(loginHref(useModelHref({ id: "tokenhub/echo-1", kind: "text" }))).toContain("next=");
  });
});
