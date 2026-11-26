import { expect, it } from "vitest";
import { defaultMonetary, PublicContextSchema, SubscriptionOAuthStartSchema, SubscriptionPoolsSchema } from "./contracts";

it("fills in missing settlement settings on earlier public context responses", () => {
  const context = {
    brand: { id: "brand-1", name: "Test Brand", api_base_url: "https://brand.example.test/v1", theme: {} },
    operator: { id: "operator-1", name: "Test Operator", kind: "platform", brand_id: "brand-1" },
    theme: {},
    availability: [],
    payment_methods: [],
  };

  expect(PublicContextSchema.parse(context).monetary).toEqual(defaultMonetary);
  const invalidMonetary = { ...defaultMonetary, atg_per_unit: "invalid" };
  expect(PublicContextSchema.safeParse({ ...context, monetary: invalidMonetary }).success).toBe(false);
});

it("keeps subscription pool states, optional account details and exact statement amounts", () => {
  const pool = {
    id: "pool-1", name: "Codex", subscription_kind: "codex", enabled: true, version: 3, browser_oauth: true,
    accounts: [
      { id: "account-1", name: "Primary", enabled: true, version: 2, status: "available" },
      { id: "account-2", name: "Backup", enabled: true, version: 4, status: "cooldown", cooldown_until: "2026-11-26T09:00:00Z" },
    ],
    latest_statement: { id: "statement-1", statement_reference: "November", amount: "19.123456789012345678", currency: "EUR" },
  };
  const page = { items: [pool], next_cursor: null };

  expect(SubscriptionPoolsSchema.parse(page)).toEqual(page);
  expect(SubscriptionPoolsSchema.safeParse({ items: [{ ...pool, subscription_kind: "unsupported" }] }).success).toBe(false);
});

it("preserves the encoded destination from an account pool OAuth start response", () => {
  const url = "https://accounts.example.test/authorize?state=one%2Btwo&redirect_uri=https%3A%2F%2Foperator.example.test%2Fcallback";

  expect(SubscriptionOAuthStartSchema.parse({ url })).toEqual({ url });
  expect(SubscriptionOAuthStartSchema.safeParse({ url: "/authorize" }).success).toBe(false);
});
