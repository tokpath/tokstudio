import { SubscriptionVendorSchema, type SubscriptionVendor } from "./contracts";

export const subscriptionVendors = {
  codex: { label: "ChatGPT / Codex", provider: "openai", operation: "responses" },
  claude: { label: "Claude Code", provider: "anthropic", operation: "chat" },
  grok: { label: "Grok", provider: "openai", operation: "responses" },
  gemini: { label: "Gemini", provider: "openai", operation: "chat" },
  antigravity: { label: "Antigravity", provider: "openai", operation: "chat" },
} as const satisfies Record<SubscriptionVendor, { label: string; provider: string; operation: string }>;

export function subscriptionVendor(kind: unknown) {
  const parsed = SubscriptionVendorSchema.safeParse(kind);
  return parsed.success ? subscriptionVendors[parsed.data] : undefined;
}
