import { describe, expect, it } from "vitest";
import {
  adapterLabel,
  catalogStatusTone,
  formatCredentialRef,
  formatMappedModels,
  formatProviderSlugs,
  filterProviderOptions,
  formatRpm,
  modelLifecycleEnabled,
  healthLabel,
  healthTone,
  protocolOptions,
  providerHref,
  providerKindLabel,
  providerStatusLabel,
  routeStatusOptions,
  routeStrategyOptions,
  statusWord,
  syncStateLabel,
} from "./catalog-admin";

describe("admin catalog labels", () => {
  it("distinguishes routing kind from model vendor", () => {
    expect(providerKindLabel("direct")).toBe("直连");
    expect(providerKindLabel("aggregator")).toBe("聚合");
    expect(providerKindLabel("")).toBe("—");
  });

  it("uses semantic tones and uppercase status words", () => {
    expect(healthTone("available")).toBe("success");
    expect(healthTone("degraded")).toBe("warn");
    expect(healthTone("maintenance")).toBe("warn");
    expect(catalogStatusTone("published")).toBe("success");
    expect(catalogStatusTone("draft")).toBe("warn");
    expect(statusWord("available")).toBe("AVAILABLE");
  });

  it("explains empty rpm, credentials, and unmapped models", () => {
    expect(formatRpm(0)).toBe("未限制");
    expect(formatRpm(30)).toBe("30");
    expect(formatCredentialRef("")).toBe("尚未配置");
    expect(formatCredentialRef("crd_123")).toBe("已配置");
    expect(formatProviderSlugs([])).toBe("尚未关联");
    expect(formatProviderSlugs(["ark-seedance", "openrouter-seedance"])).toBe(
      "ark-seedance · openrouter-seedance",
    );
    expect(formatMappedModels([])).toBe("尚未关联模型");
    expect(formatMappedModels([{ public_id: "tokenhub/echo-1" }, { public_id: "tokenhub/oem-demo" }])).toBe(
      "tokenhub/echo-1 · tokenhub/oem-demo",
    );
    expect(adapterLabel("openai")).toBe("OpenAI 兼容");
    expect(adapterLabel("test")).toBe("沙箱回声");
    expect(adapterLabel("bifrost")).toBe("Bifrost");
    expect(protocolOptions().map((item) => item.value)).toEqual([
      "openai",
      "anthropic",
      "gemini",
      "ark",
      "openrouter",
    ]);
    expect(protocolOptions("test")[0]).toEqual({ value: "test", label: "沙箱回声" });
    expect(providerStatusLabel("maintenance")).toBe("维护中");
    expect(healthLabel("available")).toBe("正常");
    expect(providerHref("echo-primary")).toBe("/admin/providers/echo-primary");
    expect(filterProviderOptions(
      [
        { id: "prd_a", name: "OpenAI", slug: "openai" },
        { id: "prd_b", name: "Echo Primary", slug: "echo-primary" },
      ],
      "echo",
    ).map((item) => item.slug)).toEqual(["echo-primary"]);
    expect(filterProviderOptions(
      [{ id: "prd_a", name: "OpenAI", slug: "openai" }],
      "OPEN",
    )).toHaveLength(1);
  });

  it("grays out listing actions that the catalog API would reject", () => {
    expect(modelLifecycleEnabled("", "")).toEqual({
      approve: false,
      reject: false,
      publish: false,
      deprecate: false,
    });
    expect(modelLifecycleEnabled("published", "published")).toEqual({
      approve: false,
      reject: false,
      publish: false,
      deprecate: true,
    });
    expect(modelLifecycleEnabled("draft", "draft")).toEqual({
      approve: true,
      reject: true,
      publish: false,
      deprecate: false,
    });
    expect(modelLifecycleEnabled("draft", "reviewed")).toEqual({
      approve: false,
      reject: true,
      publish: true,
      deprecate: false,
    });
    expect(modelLifecycleEnabled("draft", "rejected")).toEqual({
      approve: true,
      reject: false,
      publish: false,
      deprecate: false,
    });
    expect(modelLifecycleEnabled("deprecated", "published")).toEqual({
      approve: true,
      reject: false,
      publish: false,
      deprecate: false,
    });
  });

  it("maps sync states to review-queue labels", () => {
    expect(syncStateLabel("draft")).toBe("待审核");
    expect(syncStateLabel("reviewed")).toBe("已通过");
    expect(syncStateLabel("rejected")).toBe("已拒绝");
    expect(syncStateLabel("published")).toBe("已发布");
  });

  it("keeps known route strategies/statuses and prepends unknown current values", () => {
    expect(routeStrategyOptions().map((item) => item.value)).toEqual(["priority", "weight", "price", "health"]);
    expect(routeStrategyOptions("custom")[0]).toEqual({ value: "custom", label: "custom" });
    expect(routeStatusOptions().map((item) => item.value)).toEqual(["active", "inactive"]);
    expect(routeStatusOptions("paused")[0]).toEqual({ value: "paused", label: "paused" });
  });
});
