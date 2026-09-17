/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import KeysPanel, { KeysList, maskAPIKey, parseAllowlist } from "../app/console/keys-panel";
import { withZh } from "./test-i18n";

vi.mock("next/navigation", () => ({
  usePathname: () => "/app/keys",
  useSearchParams: () => new URLSearchParams(typeof window === "undefined" ? "" : window.location.search),
}));

const sampleKey = {
  id: "key_1",
  name: "default",
  prefix: "thk_abcd",
  status: "active",
  key: "thk_abcdsecret",
};

describe("KeysList", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders API Key prefix and status for the user console", () => {
    render(withZh(<KeysList items={[sampleKey]} />));
    expect(screen.getAllByText(/default/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/thk_abcd/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/已启用/).length).toBeGreaterThan(0);
    expect(screen.getAllByText("模型限制：不限制").length).toBeGreaterThan(0);
    expect(screen.getAllByText("详情").length).toBeGreaterThan(0);
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
    expect(screen.getAllByText(/模型限制：google\/gemini-flash/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/每分钟最多请求数 30/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/同时请求 1/).length).toBeGreaterThan(0);
  });

  it("renders an empty ledger when there are no keys", () => {
    render(withZh(<KeysList items={[]} />));
    expect(screen.getByText("暂无 API 密钥")).toBeTruthy();
  });

  it("reveals the full secret when asked", () => {
    render(withZh(<KeysList items={[sampleKey]} revealedIds={["key_1"]} />));
    expect(screen.getAllByText("thk_abcdsecret").length).toBeGreaterThan(0);
  });

  it("shows copy on the list so the key can be copied later", () => {
    const onCopy = vi.fn();
    render(withZh(<KeysList items={[sampleKey]} onCopy={onCopy} />));
    fireEvent.click(screen.getAllByRole("button", { name: "复制" })[0]);
    expect(onCopy).toHaveBeenCalledWith("key_1");
  });

  it("hides rotate until more actions is opened", () => {
    const onRotate = vi.fn();
    render(withZh(<KeysList items={[sampleKey]} onRotate={onRotate} />));
    expect(screen.queryByRole("button", { name: "轮换" })).toBeNull();
    fireEvent.click(screen.getAllByRole("button", { name: "更多操作" })[0]);
    expect(screen.getAllByRole("menuitem", { name: "轮换" }).length).toBeGreaterThan(0);
  });

  it("opens confirm outside the menu so a click on confirm still runs the action", async () => {
    const onDisable = vi.fn(async () => true);
    render(withZh(<KeysList items={[sampleKey]} onDisable={onDisable} />));
    fireEvent.click(screen.getAllByRole("button", { name: "更多操作" })[0]);
    fireEvent.click(screen.getAllByRole("menuitem", { name: "禁用" })[0]);
    expect(onDisable).not.toHaveBeenCalled();
    expect(screen.getByRole("heading", { name: "确认禁用" })).toBeTruthy();
    expect(screen.queryByRole("menuitem", { name: "禁用" })).toBeNull();
    fireEvent.mouseDown(document.body);
    expect(screen.getByRole("heading", { name: "确认禁用" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "确认" }));
    await waitFor(() => expect(onDisable).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(onDisable).toHaveBeenCalledWith("key_1"));
    await waitFor(() => expect(screen.queryByRole("heading", { name: "确认禁用" })).toBeNull());
  });

  it("does not call rotate, disable, or expire when the confirm is cancelled", () => {
    const onRotate = vi.fn(async () => true);
    const onDisable = vi.fn(async () => true);
    const onExpire = vi.fn(async () => true);
    render(withZh(<KeysList items={[sampleKey]} onRotate={onRotate} onDisable={onDisable} onExpire={onExpire} />));
    fireEvent.click(screen.getAllByRole("button", { name: "更多操作" })[0]);
    fireEvent.click(screen.getAllByRole("menuitem", { name: "立即过期" })[0]);
    fireEvent.click(screen.getByRole("button", { name: "取消" }));
    expect(onRotate).not.toHaveBeenCalled();
    expect(onDisable).not.toHaveBeenCalled();
    expect(onExpire).not.toHaveBeenCalled();
    expect(screen.queryByRole("heading", { name: "确认过期" })).toBeNull();
  });

  it("keeps the confirm dialog open when the action returns false", async () => {
    const onRotate = vi.fn(async () => false);
    render(withZh(<KeysList items={[sampleKey]} onRotate={onRotate} actionError="当前密钥不能轮换" />));
    fireEvent.click(screen.getAllByRole("button", { name: "更多操作" })[0]);
    fireEvent.click(screen.getAllByRole("menuitem", { name: "轮换" })[0]);
    fireEvent.click(screen.getByRole("button", { name: "确认" }));
    await waitFor(() => expect(onRotate).toHaveBeenCalledTimes(1));
    expect(screen.getByRole("heading", { name: "确认轮换" })).toBeTruthy();
    expect(screen.getByTestId("submit-status").textContent).toContain("当前密钥不能轮换");
  });

  it("activates disable from the keyboard without sending until confirm", async () => {
    const onDisable = vi.fn(async () => true);
    render(withZh(<KeysList items={[sampleKey]} onDisable={onDisable} />));
    const more = screen.getAllByRole("button", { name: "更多操作" })[0];
    more.focus();
    fireEvent.keyDown(more, { key: "Enter" });
    fireEvent.click(more);
    const disable = screen.getAllByRole("menuitem", { name: "禁用" })[0];
    disable.focus();
    fireEvent.keyDown(disable, { key: "Enter" });
    fireEvent.click(disable);
    expect(onDisable).not.toHaveBeenCalled();
    const confirm = screen.getByRole("button", { name: "确认" });
    confirm.focus();
    fireEvent.keyDown(confirm, { key: "Enter" });
    fireEvent.click(confirm);
    await waitFor(() => expect(onDisable).toHaveBeenCalledTimes(1));
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
    window.history.replaceState({}, "", "/");
  });

  it("opens the create dialog when the page is opened with create=1", async () => {
    window.history.replaceState({}, "", "/app/keys?create=1");
    render(withZh(<KeysPanel />));
    await waitFor(() => expect(screen.getByRole("heading", { name: "创建 API Key" })).toBeTruthy());
    expect(screen.getByLabelText("密钥名称")).toBeTruthy();
    expect(screen.queryByLabelText("模型限制")).toBeNull();
  });

  it("opens a create dialog instead of keeping the form on the list", () => {
    render(withZh(<KeysPanel />));
    expect(screen.getByRole("button", { name: "创建 API Key" })).toBeTruthy();
    expect(screen.queryByLabelText("密钥名称")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "创建 API Key" }));
    expect(screen.getByRole("heading", { name: "创建 API Key" })).toBeTruthy();
    expect(screen.getByLabelText("密钥名称")).toBeTruthy();
    expect(screen.getByPlaceholderText("我的聊天客户端")).toBeTruthy();
    expect(screen.queryByLabelText("每分钟最多请求数")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /^高级设置$/ }));
    expect(screen.getByPlaceholderText("搜索模型")).toBeTruthy();
    expect(screen.getByText("所有允许使用的模型")).toBeTruthy();
    expect(screen.getByLabelText("每分钟最多请求数")).toBeTruthy();
    expect(screen.getByLabelText("同时进行的请求数")).toBeTruthy();
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
    fireEvent.change(screen.getByLabelText("密钥名称"), { target: { value: "我的聊天客户端" } });
    fireEvent.click(screen.getByRole("button", { name: "创建" }));
    await waitFor(() => {
      expect(screen.getByTestId("submit-status").textContent).toContain("名称已存在");
    });
    expect(screen.getByRole("heading", { name: "创建 API Key" })).toBeTruthy();
    expect(screen.getByLabelText("密钥名称")).toHaveProperty("value", "我的聊天客户端");
  });

  it("rejects invalid request limits instead of dropping them", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true, json: async () => ({ items: [] }) }));
    vi.stubGlobal("fetch", fetchMock);
    render(withZh(<KeysPanel />));
    fireEvent.click(screen.getByRole("button", { name: "创建 API Key" }));
    fireEvent.change(screen.getByLabelText("密钥名称"), { target: { value: "我的聊天客户端" } });
    fireEvent.click(screen.getByRole("button", { name: "高级设置" }));
    fireEvent.change(screen.getByLabelText("每分钟最多请求数"), { target: { value: "abc" } });
    fireEvent.click(screen.getByRole("button", { name: "创建" }));
    await waitFor(() => {
      expect(screen.getByText("请填写正整数，或留空使用默认值")).toBeTruthy();
    });
    expect(fetchMock.mock.calls.some((call) => String(call[0]).includes("/v1/me/api-keys") && call[1]?.method === "POST")).toBe(
      false,
    );
  });

  it("shows endpoint secret example and verify after create", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.includes("/v1/me/api-keys") && init?.method === "POST") {
          return {
            ok: true,
            json: async () => ({
              item: { id: "key_9", name: "我的聊天客户端", prefix: "thk_new1", key: "thk_new1secret", status: "active" },
            }),
          };
        }
        if (url.includes("/v1/public/docs-context")) {
          return { ok: true, json: async () => ({ brand: { api_domain: "api.tokenhub.test" }, examples: { curl: "curl https://api.tokenhub.test/v1" } }) };
        }
        return { ok: true, json: async () => ({ items: [] }) };
      }),
    );
    render(withZh(<KeysPanel />));
    fireEvent.click(screen.getByRole("button", { name: "创建 API Key" }));
    fireEvent.change(screen.getByLabelText("密钥名称"), { target: { value: "我的聊天客户端" } });
    fireEvent.click(screen.getByRole("button", { name: "创建" }));
    await waitFor(() => expect(screen.getByText("接入地址")).toBeTruthy());
    expect(screen.getByText("https://api.tokenhub.test/v1")).toBeTruthy();
    expect(screen.getByTestId("key-secret").textContent).toBe("thk_new1secret");
    const sample = screen.getByTestId("key-example").textContent || "";
    expect(sample).toContain(`-H "Authorization: Bearer \${TOKENHUB_API_KEY}"`);
    expect(sample).toContain("/v1/chat/completions");
    expect(sample).toContain("Content-Type: application/json");
    expect(sample).not.toContain("thk_new1secret");
    expect(screen.getByRole("link", { name: "去快速试用" }).getAttribute("href")).toBe("/app/playground");
    expect(screen.getByRole("button", { name: "发送验证请求" })).toBeTruthy();
  });

  it("keeps a complete placeholder example when docs-context fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.includes("/v1/me/api-keys") && init?.method === "POST") {
          return {
            ok: true,
            json: async () => ({
              item: { id: "key_9", name: "我的聊天客户端", prefix: "thk_new1", key: "thk_new1secret", status: "active" },
            }),
          };
        }
        if (url.includes("/v1/public/docs-context")) {
          return { ok: false, json: async () => ({ error: { message: "docs down" } }) };
        }
        return { ok: true, json: async () => ({ items: [] }) };
      }),
    );
    render(withZh(<KeysPanel />));
    fireEvent.click(screen.getByRole("button", { name: "创建 API Key" }));
    fireEvent.change(screen.getByLabelText("密钥名称"), { target: { value: "我的聊天客户端" } });
    fireEvent.click(screen.getByRole("button", { name: "创建" }));
    await waitFor(() => expect(screen.getByTestId("key-example")).toBeTruthy());
    const sample = screen.getByTestId("key-example").textContent || "";
    expect(sample).toContain(`-H "Authorization: Bearer \${TOKENHUB_API_KEY}"`);
    expect(sample).toContain("/v1/chat/completions");
    expect(sample).toContain('"model":');
    expect(sample).not.toContain("thk_new1secret");
  });

  it("verifies with the created bearer key and does not treat a 403 as success", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.includes("/v1/me/api-keys") && init?.method === "POST") {
        return {
          ok: true,
          json: async () => ({
            item: {
              id: "key_9",
              name: "受限",
              prefix: "thk_new1",
              key: "thk_new1secret",
              status: "active",
              allowlist: ["google/gemini-flash"],
            },
          }),
        };
      }
      if (url.includes("/v1/public/models")) {
        return {
          ok: true,
          json: async () => ({
            items: [
              { id: "tokenhub/echo-1", kind: "text", status: "available" },
              { id: "google/gemini-flash", kind: "text", status: "available" },
            ],
          }),
        };
      }
      if (url.includes("/v1/public/docs-context")) {
        return { ok: true, json: async () => ({ brand: { api_domain: "api.tokenhub.test" } }) };
      }
      if (url.includes("/v1/chat/completions")) {
        return { ok: false, status: 403, json: async () => ({ error: { message: "model_not_allowed" } }) };
      }
      return { ok: true, json: async () => ({ items: [] }) };
    });
    vi.stubGlobal("fetch", fetchMock);
    render(withZh(<KeysPanel />));
    fireEvent.click(screen.getByRole("button", { name: "创建 API Key" }));
    fireEvent.change(screen.getByLabelText("密钥名称"), { target: { value: "受限" } });
    fireEvent.click(screen.getByRole("button", { name: "创建" }));
    await waitFor(() => expect(screen.getByTestId("key-example").textContent).toContain("google/gemini-flash"));
    expect(screen.getByTestId("key-example").textContent).not.toContain("tokenhub/echo-1");
    fireEvent.click(screen.getByRole("button", { name: "发送验证请求" }));
    await waitFor(() => expect(screen.getByTestId("key-verify-status").getAttribute("data-ok")).toBe("false"));
    expect(screen.getByTestId("key-verify-status").textContent).toContain("model_not_allowed");
    const verifyCall = fetchMock.mock.calls.find((call) => String(call[0]).includes("/v1/chat/completions"));
    expect(verifyCall).toBeTruthy();
    expect((verifyCall?.[1] as RequestInit).credentials).toBe("omit");
    expect((verifyCall?.[1] as RequestInit).headers).toMatchObject({ Authorization: "Bearer thk_new1secret" });
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

  it("keeps a slow key list on loading instead of an empty ledger", async () => {
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        await gate;
        return { ok: true, status: 200, json: async () => ({ items: [] }) };
      }),
    );
    render(withZh(<KeysPanel />));
    expect(screen.getByTestId("list-resource-keys").getAttribute("data-list-phase")).toBe("loading");
    expect(screen.queryByText("暂无 API 密钥")).toBeNull();
    release();
    await waitFor(() => expect(screen.getByTestId("list-resource-keys").getAttribute("data-list-phase")).toBe("empty"));
  });

  it("does not treat a 500 as 暂无 API 密钥", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: false,
        status: 500,
        json: async () => ({ error: { message: "keys down" } }),
      })),
    );
    render(withZh(<KeysPanel />));
    await waitFor(() => expect(screen.getByTestId("list-resource-keys").getAttribute("data-list-phase")).toBe("error"));
    expect(screen.getByText("keys down")).toBeTruthy();
    expect(screen.queryByText("暂无 API 密钥")).toBeNull();
  });

  it("sends disable once from the confirm dialog and keeps it open on API failure", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.includes("/v1/me/api-keys/key_1/disable") && init?.method === "POST") {
        return { ok: false, json: async () => ({ error: { message: "密钥已禁用" } }) };
      }
      return { ok: true, json: async () => ({ items: [sampleKey] }) };
    });
    vi.stubGlobal("fetch", fetchMock);
    render(withZh(<KeysPanel />));
    await screen.findAllByRole("button", { name: "更多操作" });
    fireEvent.click(screen.getAllByRole("button", { name: "更多操作" })[0]);
    fireEvent.click(screen.getAllByRole("menuitem", { name: "禁用" })[0]);
    expect(fetchMock.mock.calls.some((call) => String(call[0]).includes("/disable"))).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "取消" }));
    expect(fetchMock.mock.calls.some((call) => String(call[0]).includes("/disable"))).toBe(false);
    fireEvent.click(screen.getAllByRole("button", { name: "更多操作" })[0]);
    fireEvent.click(screen.getAllByRole("menuitem", { name: "禁用" })[0]);
    fireEvent.click(screen.getByRole("button", { name: "确认" }));
    await waitFor(() => {
      expect(screen.getByTestId("submit-status").textContent).toContain("密钥已禁用");
    });
    expect(screen.getByRole("heading", { name: "确认禁用" })).toBeTruthy();
    expect(fetchMock.mock.calls.filter((call) => String(call[0]).includes("/disable") && call[1]?.method === "POST")).toHaveLength(1);
  });
});
