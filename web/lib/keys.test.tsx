/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import KeysPanel, { KeysList, maskAPIKey, parseAllowlist } from "../app/console/keys-panel";
import { withZh } from "./test-i18n";

const sampleKey = {
  id: "key_1",
  name: "default",
  prefix: "thk_abcd",
  status: "active",
  key: "thk_abcdsecret",
};

describe("KeysList", () => {
  it("renders API Key prefix and status for the user console", () => {
    render(withZh(<KeysList items={[sampleKey]} />));
    expect(screen.getByText(/default/)).toBeTruthy();
    expect(screen.getAllByText(/thk_abcd/).length).toBeGreaterThan(0);
    expect(screen.getByText(/active/)).toBeTruthy();
    expect(screen.getByText(/模型白名单：不限制/)).toBeTruthy();
    expect(screen.queryByText("thk_abcdsecret")).toBeNull();
  });

  it("renders a model allowlist and RPM", () => {
    render(
      withZh(
        <KeysList
          items={[
            {
              id: "key_2",
              name: "gemini-only",
              prefix: "thk_gem1",
              status: "active",
              rpm_limit: 30,
              concurrency_limit: 1,
              allowlist: ["google/gemini-flash"],
            },
          ]}
        />,
      ),
    );
    expect(screen.getByText(/模型白名单：google\/gemini-flash/)).toBeTruthy();
    expect(screen.getByText(/RPM 30/)).toBeTruthy();
    expect(screen.getByText(/并发 1/)).toBeTruthy();
  });

  it("renders an empty ledger when there are no keys", () => {
    render(withZh(<KeysList items={[]} />));
    expect(screen.getByText("暂无 API 密钥")).toBeTruthy();
  });

  it("reveals the full secret when asked", () => {
    render(withZh(<KeysList items={[sampleKey]} revealedIds={["key_1"]} />));
    expect(screen.getByText("thk_abcdsecret")).toBeTruthy();
  });

  it("shows copy on the list so the key can be copied later", () => {
    const onCopy = vi.fn();
    render(withZh(<KeysList items={[sampleKey]} onCopy={onCopy} />));
    fireEvent.click(screen.getByRole("button", { name: "复制" }));
    expect(onCopy).toHaveBeenCalledWith("key_1");
  });

  it("parses comma-separated allowlists", () => {
    expect(parseAllowlist(" tokenhub/echo-1 , google/gemini-flash，tokenhub/echo-1 ")).toEqual([
      "tokenhub/echo-1",
      "google/gemini-flash",
      "tokenhub/echo-1",
    ]);
    expect(parseAllowlist("")).toEqual([]);
  });

  it("masks secrets behind the prefix", () => {
    expect(maskAPIKey("thk_abcd", "thk_abcdsecret")).toBe("thk_abcd••••••••");
    expect(maskAPIKey("thk_abcd", "thk_abcdsecret", true)).toBe("thk_abcdsecret");
  });
});

describe("KeysPanel", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: true,
        json: async () => ({ items: [] }),
      })),
    );
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("opens a create dialog instead of keeping the form on the list", () => {
    render(withZh(<KeysPanel />));
    expect(screen.getByRole("button", { name: "创建 API Key" })).toBeTruthy();
    expect(screen.queryByLabelText("模型白名单")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "创建 API Key" }));
    expect(screen.getByRole("heading", { name: "创建 API Key" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "模型白名单" })).toBeTruthy();
    expect(screen.getByLabelText("模型白名单")).toBeTruthy();
    expect(screen.getByLabelText("并发限额")).toBeTruthy();
  });

  it("keeps the create dialog open and shows the API error next to submit", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        if (String(input).includes("/v1/me/api-keys") && init?.method === "POST") {
          return {
            ok: false,
            json: async () => ({ error: { message: "名称已存在" } }),
          };
        }
        return { ok: true, json: async () => ({ items: [] }) };
      }),
    );
    render(withZh(<KeysPanel />));
    fireEvent.click(screen.getByRole("button", { name: "创建 API Key" }));
    fireEvent.click(screen.getByRole("button", { name: "创建" }));
    await waitFor(() => {
      expect(screen.getByTestId("submit-status").textContent).toContain("名称已存在");
    });
    expect(screen.getByRole("heading", { name: "创建 API Key" })).toBeTruthy();
    expect(screen.getByLabelText("API Key 名称")).toHaveProperty("value", "default");
  });

  it("does not claim copy success when clipboard write fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.endsWith("/copy") && init?.method === "POST") {
          return { ok: true, json: async () => ({ ok: true }) };
        }
        return {
          ok: true,
          json: async () => ({ items: [sampleKey] }),
        };
      }),
    );
    vi.stubGlobal("navigator", {
      clipboard: {
        writeText: vi.fn(async () => {
          throw new Error("denied");
        }),
      },
    });
    render(withZh(<KeysPanel />));
    await waitFor(() => expect(screen.getAllByText(/default/).length).toBeGreaterThan(0));
    fireEvent.click(screen.getAllByRole("button", { name: "复制" })[0]!);
    await waitFor(() => {
      expect(screen.getByTestId("submit-status").textContent).toContain("thk_abcdsecret");
      expect(screen.getByTestId("submit-status").textContent).toContain("未能写入剪贴板");
    });
  });
});
