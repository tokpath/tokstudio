import { describe, expect, it } from "vitest";
import {
  dimToKeyBuckets,
  filterUsage,
  groupUsageByAPIKey,
  groupUsageByDay,
  groupUsageByModel,
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
    occurred_at: "2026-08-29T10:00:00.000Z",
  },
  {
    id: "usg_b",
    api_key_id: "key_alpha",
    public_model_id: "tokenhub/echo-1",
    prompt_tokens: 8,
    completion_tokens: 4,
    customer_amount_minor: 16,
    occurred_at: "2026-08-29T18:00:00.000Z",
  },
  {
    id: "usg_c",
    api_key_id: "key_beta",
    public_model_id: "tokenhub/other",
    unit_usage: { prompt_tokens: 8, completion_tokens: 4 },
    customer_amount_minor: 16,
    occurred_at: "2026-08-30T09:00:00.000Z",
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

  it("groups events into daily and model chart series", () => {
    expect(groupUsageByDay(rows)).toEqual([
      { day: "2026-08-29", requests: 2, revenue_minor: 80 },
      { day: "2026-08-30", requests: 1, revenue_minor: 16 },
    ]);
    expect(groupUsageByModel(rows)[0]).toMatchObject({ key: "tokenhub/echo-1", requests: 2, revenue_minor: 80 });
  });
});

it("keeps actual usage but excludes refunded and unconfirmed spend in every grouping", () => {
  const mixed = [
    { ...rows[0], state: "confirmed" },
    { ...rows[1], state: "voided" },
    { ...rows[2], state: "pending_reconciliation" },
  ];
  expect(summarizeUsage(mixed)).toMatchObject({ requests: 3, prompt: 56, completion: 20, amount: 64 });
  expect(groupUsageByAPIKey(mixed).map(r => r.amount)).toEqual([64, 0]);
  expect(groupUsageByDay(mixed).map(r => r.revenue_minor)).toEqual([64, 0]);
  expect(groupUsageByModel(mixed).map(r => r.revenue_minor)).toEqual([64, 0]);
  expect(mixed[1].customer_amount_minor).toBe(16);
});
