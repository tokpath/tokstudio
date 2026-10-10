import { describe, expect, it } from "vitest";
import { publicRedirectHref } from "./public-redirect";

describe("public compatibility redirects", () => {
  it("keeps the model task, invitation and legal return path", () => {
    const href = publicRedirectHref("/models?tab=agent", { model: "vendor/model-v2", promo: "INVITE", next: "/app/keys?model=vendor%2Fmodel-v2" });
    const parsed = new URL(href, "https://brand.test");
    expect(parsed.pathname).toBe("/models/vendor/model-v2");
    expect(parsed.searchParams.get("tab")).toBe("agent");
    expect(parsed.searchParams.get("promo")).toBe("INVITE");
    expect(parsed.searchParams.get("next")).toBe("/app/keys?model=vendor%2Fmodel-v2");
  });
  it("keeps the actual model intent for integrations and the original finder path", () => {
    expect(publicRedirectHref("/docs/integrations", { model: "vendor/model", promotion_code: "INVITE" })).toBe("/docs/integrations?promotion_code=INVITE&model=vendor%2Fmodel");
    expect(publicRedirectHref("/models", { model: "other" }, "original-model")).toBe("/models/original-model");
  });
  it("rejects open returns and model path traversal", () => {
    expect(publicRedirectHref("/models", { model: "../login", next: "//external.test/login" })).toBe("/models");
    expect(publicRedirectHref("/models", { model: "vendor/../login", next: "https://external.test" })).toBe("/models");
  });
  it("retains catalog filters without overriding the alias capability", () => {
    expect(publicRedirectHref("/models?kind=image", { kind: "video", vendor: "vendor", q: ["first", "second"] })).toBe("/models?kind=image&vendor=vendor&q=first");
  });
});
