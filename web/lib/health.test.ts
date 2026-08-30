import { describe, expect, it } from "vitest";
import { formatHealthCurl, listHealthFields } from "./health";

describe("listHealthFields", () => {
  it("keeps the published field order", () => {
    const keys = listHealthFields({
      version: "0.1.0-m0",
      request_id: "req_1",
      status: "ok",
      service: "tokenhub-api",
    }).map((row) => row.key);
    expect(keys).toEqual(["status", "service", "version", "request_id"]);
  });

  it("treats a fetch failure as one hold row", () => {
    expect(listHealthFields({ error: "无法连接 API" })).toEqual([
      { key: "api", label: "API", value: "unreachable" },
    ]);
  });
});

describe("formatHealthCurl", () => {
  it("uses the brand base URL and never embeds a key", () => {
    const snippet = formatHealthCurl("http://localhost:8080/");
    expect(snippet).toBe("curl -sS http://localhost:8080/healthz");
    expect(snippet).not.toMatch(/sk-|Bearer|api[_-]?key/i);
  });
});
