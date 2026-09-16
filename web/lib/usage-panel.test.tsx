/** @vitest-environment jsdom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import UsagePanel from "../app/console/usage-panel";
import { withZh } from "./test-i18n";

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
});
