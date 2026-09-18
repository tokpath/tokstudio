/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import UsagePanel from "../app/console/usage-panel";
import { withZh } from "./test-i18n";

vi.mock("@/components/usage-charts", () => ({
  UsageCharts: ({ events }: { events: unknown[] }) => (
    <div data-testid="usage-charts-scope" data-event-count={events.length} />
  ),
}));

vi.mock("next/navigation", () => ({
  usePathname: () => "/app/usage",
}));

describe("UsagePanel", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/v1/me/usage")) {
          return { ok: true, status: 200, json: async () => ({ items: [], keys: [], models: [] }) };
        }
        if (url.includes("/v1/me/api-keys")) {
          return { ok: true, status: 200, json: async () => ({ items: [] }) };
        }
        return { ok: true, status: 200, json: async () => ({}) };
      }),
    );
  });

  it("shows empty usage after a successful zero-row load", async () => {
    render(withZh(<UsagePanel />));
    await waitFor(() => expect(screen.getByTestId("list-resource-usage").getAttribute("data-list-phase")).toBe("empty"));
    expect(screen.getByText("暂无用量")).toBeTruthy();
  });

  it("does not treat a 500 as no usage", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (String(input).includes("/v1/me/usage")) {
          return { ok: false, status: 500, json: async () => ({ error: { message: "usage upstream timeout" } }) };
        }
        return { ok: true, status: 200, json: async () => ({ items: [] }) };
      }),
    );
    render(withZh(<UsagePanel />));
    await waitFor(() => expect(screen.getByTestId("list-resource-usage").getAttribute("data-list-phase")).toBe("error"));
    expect(screen.getByText("usage upstream timeout")).toBeTruthy();
    expect(screen.queryByText("暂无用量")).toBeNull();
  });

  it("keeps a hanging usage request on loading", async () => {
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        await gate;
        return { ok: true, status: 200, json: async () => ({ items: [], keys: [], models: [] }) };
      }),
    );
    render(withZh(<UsagePanel />));
    expect(screen.getByTestId("list-resource-usage").getAttribute("data-list-phase")).toBe("loading");
    expect(screen.queryByText("暂无用量")).toBeNull();
    release();
    await waitFor(() => expect(screen.getByTestId("list-resource-usage").getAttribute("data-list-phase")).toBe("empty"));
  });

  it("keeps summary, charts, and per-key table on the same loaded 100-row window", async () => {
    const echoLoaded = Array.from({ length: 80 }, (_, i) =>
      usageEvent(`echo-${i}`, "tokenhub/echo-1", "key_alpha", 1_000_000),
    );
    const otherLoaded = Array.from({ length: 20 }, (_, i) =>
      usageEvent(`other-${i}`, "tokenhub/other", "key_beta", 2_000_000),
    );
    const unfiltered = [...echoLoaded, ...otherLoaded];
    const echoWindow = Array.from({ length: 100 }, (_, i) =>
      usageEvent(`echo-all-${i}`, "tokenhub/echo-1", "key_alpha", 1_000_000),
    );
    const ledgerKeys = [{ key: "key_alpha", requests: 999, revenue_minor: 999_000_000 }];
    const ledgerModels = [{ key: "tokenhub/echo-1" }, { key: "tokenhub/other" }];

    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/v1/me/api-keys")) {
          return {
            ok: true,
            status: 200,
            json: async () => ({
              items: [
                { id: "key_alpha", name: "alpha" },
                { id: "key_beta", name: "beta" },
              ],
            }),
          };
        }
        if (url.includes("/v1/me/usage")) {
          const model = new URL(url, "http://local").searchParams.get("public_model_id");
          const items = model === "tokenhub/echo-1" ? echoWindow : unfiltered;
          return {
            ok: true,
            status: 200,
            json: async () => ({ items, keys: ledgerKeys, models: ledgerModels }),
          };
        }
        return { ok: true, status: 200, json: async () => ({}) };
      }),
    );

    render(withZh(<UsagePanel />));
    await waitFor(() => expect(screen.getByTestId("usage-stat-requests").textContent).toBe("100"));
    expect(screen.getByTestId("usage-stat-spend").textContent).toBe("$120.00");
    expect(screen.getByTestId("usage-charts-scope").getAttribute("data-event-count")).toBe("100");
    expect(screen.getByTestId("usage-key-amount-key_alpha").textContent).toBe("$80.00");
    expect(screen.getByTestId("usage-key-amount-key_beta").textContent).toBe("$40.00");
    expect(screen.queryByText("999")).toBeNull();
    expect(screen.queryByText("999000000")).toBeNull();
    expect(screen.queryByText("$999.00")).toBeNull();
    expect(screen.getByText(/数字、图表和按密钥汇总都来自当前筛选下最近 100 条已加载记录，不是完整周期账单。/)).toBeTruthy();

    fireEvent.change(screen.getByLabelText("按模型筛选"), { target: { value: "tokenhub/echo-1" } });
    await waitFor(() => expect(screen.getByTestId("usage-stat-spend").textContent).toBe("$100.00"));
    expect(screen.getByTestId("usage-stat-requests").textContent).toBe("100");
    expect(screen.getByTestId("usage-charts-scope").getAttribute("data-event-count")).toBe("100");
    expect(screen.getByTestId("usage-key-amount-key_alpha").textContent).toBe("$100.00");
    expect(screen.queryByTestId("usage-key-amount-key_beta")).toBeNull();
    expect(screen.queryByText("$80.00")).toBeNull();
    expect(screen.queryByText("$120.00")).toBeNull();
    expect(screen.queryByText("999000000")).toBeNull();
  });
});

function usageEvent(id: string, model: string, key: string, amount: number) {
  return {
    id,
    public_model_id: model,
    api_key_id: key,
    prompt_tokens: 1,
    completion_tokens: 1,
    customer_amount_minor: amount,
    occurred_at: "2026-09-16T12:00:00.000Z",
  };
}
