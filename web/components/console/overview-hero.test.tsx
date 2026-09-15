/** @vitest-environment jsdom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { OverviewHero } from "@/components/console/overview-hero";
import { withZh } from "@/lib/test-i18n";

function jsonOk(body: unknown) {
  return { ok: true, json: async () => body };
}

describe("OverviewHero first-run vs returning", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/v1/me/balance")) {
          return jsonOk({ balance: { available: "12.00", reserved: "0" } });
        }
        if (url.includes("/v1/me/api-keys")) {
          return jsonOk({ items: [] });
        }
        if (url.includes("/v1/me/usage")) {
          return jsonOk({ items: [] });
        }
        return jsonOk({});
      }),
    );
  });

  it("shows first-run tasks that open playground without requiring a key", async () => {
    render(withZh(<OverviewHero />));
    await waitFor(() => expect(screen.getByRole("heading", { name: "第一次使用" })).toBeTruthy());
    expect(screen.getByRole("heading", { name: "直接体验模型" })).toBeTruthy();
    expect(screen.getByText("选择模型 → 输入需求 → 获得结果")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "接入自己的应用" })).toBeTruthy();
    const tryLinks = screen.getAllByRole("link", { name: "去快速试用" });
    expect(tryLinks[0]?.getAttribute("href")).toBe("/app/playground");
    const createLinks = screen.getAllByRole("link", { name: "创建 API Key" });
    expect(createLinks[0]?.getAttribute("href")).toBe("/app/keys?create=1");
    expect(screen.queryByRole("heading", { name: "快捷入口" })).toBeNull();
  });

  it("offers top-up when available balance is zero", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/v1/me/balance")) {
          return jsonOk({ balance: { available: "0", reserved: "0" } });
        }
        if (url.includes("/v1/me/api-keys")) {
          return jsonOk({ items: [] });
        }
        if (url.includes("/v1/me/usage")) {
          return jsonOk({ items: [] });
        }
        return jsonOk({});
      }),
    );
    render(withZh(<OverviewHero />));
    await waitFor(() => expect(screen.getByText("当前可用余额不足，体验前请先充值。")).toBeTruthy());
    expect(screen.getAllByRole("link", { name: "充值余额" }).length).toBeGreaterThan(0);
  });

  it("restores shortcuts after real usage exists", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/v1/me/balance")) {
          return jsonOk({ balance: { available: "12.00", reserved: "0" } });
        }
        if (url.includes("/v1/me/api-keys")) {
          return jsonOk({ items: [{ id: "key_1" }] });
        }
        if (url.includes("/v1/me/usage")) {
          return jsonOk({ items: [{ public_model_id: "tokenhub/echo-1", state: "succeeded", request_id: "req_1" }] });
        }
        return jsonOk({});
      }),
    );
    render(withZh(<OverviewHero />));
    await waitFor(() => expect(screen.getByRole("heading", { name: "快捷入口" })).toBeTruthy());
    expect(screen.queryByRole("heading", { name: "第一次使用" })).toBeNull();
    expect(screen.getByRole("link", { name: "创建 API Key" }).getAttribute("href")).toBe("/app/keys?create=1");
  });
});
