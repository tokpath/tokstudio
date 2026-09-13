import { describe, expect, it } from "vitest";
import {
  adapterLabel,
  catalogStatusTone,
  formatCredentialRef,
  formatMappedModels,
  formatProviderSlugs,
  formatRpm,
  healthLabel,
  healthTone,
  providerHref,
  providerKindLabel,
  providerStatusLabel,
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
    expect(providerStatusLabel("maintenance")).toBe("维护中");
    expect(healthLabel("available")).toBe("正常");
    expect(providerHref("echo-primary")).toBe("/admin/providers/echo-primary");
  });

  it("maps sync states to review-queue labels", () => {
    expect(syncStateLabel("draft")).toBe("待审核");
    expect(syncStateLabel("reviewed")).toBe("已通过");
    expect(syncStateLabel("rejected")).toBe("已拒绝");
    expect(syncStateLabel("published")).toBe("已发布");
  });
});
