import { describe, expect, it } from "vitest";
import {
  dimToKeyBuckets,
  filterUsage,
  groupUsageByAPIKey,
  keyLabel,
  shortKeyRef,
  summarizeUsage,
  uniqueModels,
  usageTokens,
} from "./usage";

const rows = [
  {
    id: "usg_a",
    api_key_id: "key_alpha",
    public_model_id: "tokenhub/echo-1",
    prompt_tokens: 40,
    completion_tokens: 12,
    customer_amount_minor: 64,
  },
  {
    id: "usg_b",
    api_key_id: "key_alpha",
    public_model_id: "tokenhub/echo-1",
    prompt_tokens: 8,
    completion_tokens: 4,
    customer_amount_minor: 16,
  },
  {
    id: "usg_c",
    api_key_id: "key_beta",
    public_model_id: "tokenhub/other",
    unit_usage: { prompt_tokens: 8, completion_tokens: 4 },
    customer_amount_minor: 16,
  },
];

describe("usage grouping", () => {
  it("reads tokens from top-level or unit_usage", () => {
    expect(usageTokens(rows[2])).toEqual({ prompt: 8, completion: 4, reasoning: 0 });
  });

  it("summarizes requests, tokens, and spend", () => {
    expect(summarizeUsage(rows)).toEqual({
      requests: 3,
      prompt: 56,
      completion: 20,
      reasoning: 0,
      amount: 96,
    });
  });

  it("groups by API key like Sub2API's key split", () => {
    const grouped = groupUsageByAPIKey(rows);
    expect(grouped.map((row) => row.api_key_id)).toEqual(["key_alpha", "key_beta"]);
    expect(grouped[0]).toMatchObject({ requests: 2, prompt: 48, amount: 80 });
  });

  it("filters by key and model", () => {
    expect(filterUsage(rows, { apiKeyId: "key_beta" })).toHaveLength(1);
    expect(uniqueModels(rows)).toEqual(["tokenhub/echo-1", "tokenhub/other"]);
  });

  it("labels keys by name when the list is loaded", () => {
    expect(shortKeyRef("key_abcdefghijklmnopqrst")).toContain("…");
    expect(keyLabel("key_alpha", [{ id: "key_alpha", name: "alpha" }])).toBe("alpha");
    expect(dimToKeyBuckets([{ key: "key_alpha", requests: 2, revenue_minor: 80, prompt_tokens: 48 }])[0].api_key_id).toBe(
      "key_alpha",
    );
  });
});
