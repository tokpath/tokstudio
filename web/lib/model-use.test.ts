/** @vitest-environment node */
import { execFile } from "node:child_process";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import { promisify } from "node:util";
import { describe, expect, it } from "vitest";
import { loginHref } from "./login-next";
import { CURL_BEARER_HEADER, exampleCurl, examplePath, modelEntry, useModelHref } from "./model-use";

const execFileAsync = promisify(execFile);
const virtualKey = "thk_virtual_test";

async function captureExampleCurl(curl: string, httpsHost: string) {
  const captured = { method: "", url: "", auth: "", contentType: "", body: "" };
  const server = createServer((req, res) => {
    captured.method = req.method || "";
    captured.url = req.url || "";
    captured.auth = String(req.headers.authorization || "");
    captured.contentType = String(req.headers["content-type"] || "");
    const chunks: Buffer[] = [];
    req.on("data", (chunk) => chunks.push(Buffer.from(chunk)));
    req.on("end", () => {
      captured.body = Buffer.concat(chunks).toString("utf8");
      res.statusCode = 204;
      res.end();
    });
  });
  await new Promise<void>((resolve) => {
    server.listen(0, "127.0.0.1", resolve);
  });
  const { port } = server.address() as AddressInfo;
  const rewritten = curl.replace(`https://${httpsHost}`, `http://127.0.0.1:${port}`);
  try {
    await execFileAsync("bash", ["-lc", rewritten], {
      env: { ...process.env, TOKENHUB_API_KEY: virtualKey },
    });
  } finally {
    await new Promise<void>((resolve, reject) => {
      server.close((err) => (err ? reject(err) : resolve()));
    });
  }
  return captured;
}

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

  it("quotes the bearer header so a virtual key expands when curl runs", async () => {
    const curl = exampleCurl("google/gemini-flash", "/v1/chat/completions", "api.test");
    expect(curl).toContain(CURL_BEARER_HEADER);
    expect(curl).not.toContain("'Authorization:");
    expect(curl).not.toMatch(/thk_/);
    const captured = await captureExampleCurl(curl, "api.test");
    expect(captured.method).toBe("POST");
    expect(captured.url).toBe("/v1/chat/completions");
    expect(captured.auth).toBe(`Bearer ${virtualKey}`);
    expect(captured.auth).not.toContain("$TOKENHUB_API_KEY");
    expect(captured.contentType).toMatch(/application\/json/);
    expect(JSON.parse(captured.body)).toEqual({
      model: "google/gemini-flash",
      messages: [{ role: "user", content: "hi" }],
    });
  });

  it("posts Responses and Messages bodies on their own paths", async () => {
    const responses = await captureExampleCurl(exampleCurl("openai/gpt", "/v1/responses", "api.test"), "api.test");
    expect(responses.method).toBe("POST");
    expect(responses.url).toBe("/v1/responses");
    expect(responses.auth).toBe(`Bearer ${virtualKey}`);
    expect(JSON.parse(responses.body)).toEqual({ model: "openai/gpt", input: "hi" });

    const messages = await captureExampleCurl(exampleCurl("anthropic/claude", "/v1/messages", "api.test"), "api.test");
    expect(messages.url).toBe("/v1/messages");
    expect(messages.auth).toBe(`Bearer ${virtualKey}`);
    expect(JSON.parse(messages.body)).toEqual({
      model: "anthropic/claude",
      max_tokens: 32,
      messages: [{ role: "user", content: "hi" }],
    });
  });
});

describe("guest start href", () => {
  it("keeps login next on the matching entry", () => {
    expect(loginHref(useModelHref({ id: "tokenhub/echo-1", kind: "text" }))).toContain("next=");
  });
});
