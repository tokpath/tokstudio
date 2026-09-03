import { describe, expect, it } from "vitest";
import {
  catalogStatusTone,
  formatCredentialRef,
  formatProviderSlugs,
  formatRpm,
  healthTone,
  providerKindLabel,
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
    expect(formatCredentialRef("")).toBe("未配置");
    expect(formatCredentialRef("crd_123")).toBe("crd_123");
    expect(formatProviderSlugs([])).toBe("未挂载");
    expect(formatProviderSlugs(["ark-seedance", "openrouter-seedance"])).toBe(
      "ark-seedance · openrouter-seedance",
    );
  });

  it("maps sync states to review-queue labels", () => {
    expect(syncStateLabel("draft")).toBe("待审核");
    expect(syncStateLabel("reviewed")).toBe("已通过");
    expect(syncStateLabel("rejected")).toBe("已拒绝");
    expect(syncStateLabel("published")).toBe("已发布");
  });
});
