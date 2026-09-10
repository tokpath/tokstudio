import { describe, expect, it } from "vitest";
import {
  MISSING_UPSTREAM,
  badgeLabel,
  factsJSON,
  forbidsFakeUpstream,
  fromDiffRow,
  hasCompleteUpstreamFacts,
  isSandboxFact,
  missingUpstreamLabel,
  truncateFact,
} from "./upstream-facts";

describe("W1-① upstream facts badge", () => {
  it("requires provider, model, and request_id before showing facts", () => {
    expect(hasCompleteUpstreamFacts({ provider: "prd_echo", model: "echo-up", request_id: "req_1" })).toBe(true);
    expect(hasCompleteUpstreamFacts({ provider: "prd_echo", model: "echo-up" })).toBe(false);
    expect(hasCompleteUpstreamFacts({ request_id: "req_1" })).toBe(false);
    expect(hasCompleteUpstreamFacts({})).toBe(false);
    expect(missingUpstreamLabel()).toBe(MISSING_UPSTREAM);
  });

  it("never invents missing fields and keeps the locked empty copy", () => {
    expect(badgeLabel({ request_id: "req_only" })).toBe("缺上游元数据");
    expect(badgeLabel({})).toBe("缺上游元数据");
    expect(factsJSON({ request_id: "req_1", provider: "" })).toEqual({ request_id: "req_1" });
    expect(factsJSON({})).toEqual({});
    expect(forbidsFakeUpstream(badgeLabel({ request_id: "req_1" }))).toBe(true);
    expect(badgeLabel({ request_id: "req_1" })).not.toMatch(/openai|gpt-4|estimate/i);
  });

  it("truncates complete facts for the muted pill", () => {
    const label = badgeLabel({
      provider: "prd_echo_primary_long",
      model: "echo-upstream-model",
      request_id: "req_abcdef123456",
    });
    expect(label).toContain("prd_echo_pr…");
    expect(label).toContain("echo-upstre…");
    expect(label).toContain("req_abcdef1…");
    expect(truncateFact("short")).toBe("short");
  });

  it("maps TokenHub row fields without promoting public_model_id", () => {
    const facts = fromDiffRow({
      provider_id: "prd_echo",
      upstream_model_id: "echo-upstream",
      request_id: "req_ok",
      fact_source: "sandbox",
    });
    expect(facts.provider).toBe("prd_echo");
    expect(facts.model).toBe("echo-upstream");
    expect(isSandboxFact(facts)).toBe(true);
  });
});
