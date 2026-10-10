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

  it("shows actual usage above the limit and signed remaining after occupancy", () => {
    render(withZh(<KeysList items={[{...sampleKey, budget_limit_minor:100000, budget_used_minor:120000, budget_reserved_minor:30000}]} />));
    expect(screen.getAllByText(/已用 0.120000 USD/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/剩余 -0.050000 USD/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/占用 0.030000 USD/).length).toBeGreaterThan(0);
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

  it("recovers an unknown creation with the original operation and frozen payload", async () => {
    const payloads: string[]=[];
    vi.stubGlobal("fetch",vi.fn(async(input,init)=>{
      if(String(input).endsWith("/v1/me/api-keys")&&init?.method==="POST"){
        payloads.push(String(init.body));
        if(payloads.length===1)throw new TypeError("connection lost after commit");
        return {ok:true,json:async()=>({item:sampleKey})};
      }
      return {ok:true,json:async()=>({items:[]})};
    }));
    render(withZh(<KeysPanel/>));
    fireEvent.click(screen.getByRole("button",{name:"创建 API Key"}));
    fireEvent.change(screen.getByLabelText("密钥名称"),{target:{value:"Stable operation"}});
    fireEvent.click(screen.getByRole("button",{name:"创建"}));
    await screen.findByRole("button",{name:"恢复此次创建"});
    expect(screen.getByLabelText("密钥名称").closest("fieldset")?.disabled).toBe(true);
    expect(JSON.parse(payloads[0]).operation_id).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"恢复此次创建"}));
    await screen.findByTestId("key-secret");
    expect(payloads).toHaveLength(2);expect(payloads[1]).toBe(payloads[0]);
  });

  it("keeps a confirmed creation visible when list readback fails",async()=>{
    let committed=false;
    vi.stubGlobal("fetch",vi.fn(async(input,init)=>{
      if(String(input).includes("/v1/me/api-keys")){
        if(init?.method==="POST"){committed=true;return {ok:true,json:async()=>({item:sampleKey})};}
        if(committed)return {ok:false,status:500,json:async()=>({error:{message:"readback unavailable"}})};
      }
      return {ok:true,json:async()=>({items:[]})};
    }));
    render(withZh(<KeysPanel/>));fireEvent.click(screen.getByRole("button",{name:"创建 API Key"}));
    fireEvent.change(screen.getByLabelText("密钥名称"),{target:{value:"Confirmed"}});
    fireEvent.click(screen.getByRole("button",{name:"创建"}));
    expect((await screen.findByTestId("key-secret")).textContent).toBe(sampleKey.key);
    await waitFor(()=>expect(screen.getByTestId("list-resource-keys").getAttribute("data-list-phase")).toBe("error"));
    expect(screen.queryByRole("button",{name:"恢复此次创建"})).toBeNull();
  });

  it("shows non-dialog copy API failures",async()=>{
    vi.stubGlobal("fetch",vi.fn(async(input,init)=>String(input).endsWith("/copy")&&init?.method==="POST"?{ok:false,status:403,json:async()=>({error:{message:"copy denied"}})}:{ok:true,json:async()=>({items:[sampleKey]})}));
    render(withZh(<KeysPanel/>));await screen.findAllByRole("button",{name:"复制"});
    fireEvent.click(screen.getAllByRole("button",{name:"复制"})[0]);
    await waitFor(()=>expect(screen.getByTestId("submit-status").textContent).toContain("copy denied"));
  });

  it("opens the create dialog when the page is opened with create=1", async () => {
    window.history.replaceState({}, "", "/app/keys?create=1");
    render(withZh(<KeysPanel />));
    await waitFor(() => expect(screen.getByRole("heading", { name: "创建 API Key" })).toBeTruthy());
    expect(screen.getByLabelText("密钥名称")).toBeTruthy();
    expect(screen.queryByLabelText("模型限制")).toBeNull();
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

  it("keeps draft B and still lists A when A's create returns late", async () => {
    let listed: Array<{ id: string; name: string; prefix: string; status: string }> = [];
    const createA = deferredJson();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.includes("/v1/me/api-keys") && init?.method === "POST") {
          const name = JSON.parse(String(init.body || "{}")).name;
          if (name === "密钥A") {
            return createA.promise;
          }
          return {
            ok: true,
            json: async () => ({
              item: { id: "key_b", name: "密钥B", prefix: "thk_b", key: "thk_bsecret", status: "active" },
            }),
          };
        }
        if (url.includes("/v1/public/docs-context")) {
          return { ok: true, json: async () => ({ brand: { api_domain: "api.tokenhub.test" } }) };
        }
        if (url.includes("/v1/public/models")) {
          return { ok: true, json: async () => ({ items: [] }) };
        }
        return { ok: true, json: async () => ({ items: listed }) };
      }),
    );
    render(withZh(<KeysPanel />));
    fireEvent.click(screen.getAllByRole("button", { name: "创建 API Key" })[0]);
    fireEvent.change(screen.getByLabelText("密钥名称"), { target: { value: "密钥A" } });
    fireEvent.click(screen.getByRole("button", { name: "创建" }));
    await waitFor(() => expect(vi.mocked(fetch).mock.calls.some(([url, init]) =>
      String(url).includes("/v1/me/api-keys") && init?.method === "POST")).toBe(true));
    fireEvent.click(screen.getByRole("button", { name: "关闭" }));
    await waitFor(() => expect(screen.queryByRole("heading", { name: "创建 API Key" })).toBeNull());
    fireEvent.click(screen.getAllByRole("button", { name: "创建 API Key" })[0]);
    await waitFor(() => expect(screen.getByRole("heading", { name: "创建 API Key" })).toBeTruthy());
    fireEvent.change(screen.getByLabelText("密钥名称"), { target: { value: "密钥B" } });
    listed = [{ id: "key_a", name: "密钥A", prefix: "thk_a", status: "active" }];
    createA.resolve(true, {
      item: { id: "key_a", name: "密钥A", prefix: "thk_a", key: "thk_asecret", status: "active" },
    });
    await waitFor(() => expect(screen.getAllByText("密钥A").length).toBeGreaterThan(0));
    expect(screen.getByRole("heading", { name: "创建 API Key" })).toBeTruthy();
    expect(screen.getByLabelText("密钥名称")).toHaveProperty("value", "密钥B");
    expect(screen.queryByRole("heading", { name: "API Key 已创建" })).toBeNull();
    expect(screen.queryByTestId("key-secret")).toBeNull();
  });

  it("shows all three limits directly and keeps RPM/concurrency in details", async()=>{
    vi.stubGlobal("fetch",vi.fn(async()=>({ok:true,json:async()=>({items:[]})})));
    render(withZh(<KeysPanel/>));fireEvent.click(screen.getByRole("button",{name:"创建 API Key"}));
    expect(screen.getByText("可用模型")).toBeTruthy();expect(screen.getByText("USD 消耗上限")).toBeTruthy();expect(screen.getByText("有效期")).toBeTruthy();
    expect(screen.getByLabelText("每分钟最多请求数").closest("details")?.open).toBe(false);
  });
  it("validates empty selected models, exact USD and advanced limits without submitting",async()=>{
    const fetchMock=vi.fn(async()=>({ok:true,json:async()=>({items:[]})}));vi.stubGlobal("fetch",fetchMock);
    render(withZh(<KeysPanel/>));fireEvent.click(screen.getByRole("button",{name:"创建 API Key"}));fireEvent.change(screen.getByLabelText("密钥名称"),{target:{value:"work"}});
    fireEvent.click(screen.getByLabelText("指定模型"));fireEvent.click(screen.getByRole("button",{name:"创建"}));
    await waitFor(()=>expect(screen.getByTestId("submit-status").textContent).toContain("请填写名称、指定模型"));
    fireEvent.click(screen.getByLabelText("所有可用模型"));fireEvent.click(screen.getByLabelText("设置累计总上限"));fireEvent.change(screen.getByLabelText("USD"),{target:{value:"0.0000001"}});fireEvent.click(screen.getByRole("button",{name:"创建"}));
    expect(fetchMock.mock.calls.some(call=>(call as unknown as [unknown,RequestInit])[1]?.method==="POST")).toBe(false);
  });
  it("creates without balance or Agent selection, sends atomic limits and never automatically bills",async()=>{
    const fetchMock=vi.fn(async(input:RequestInfo|URL,init?:RequestInit)=>({ok:true,json:async()=>init?.method==="POST"?{item:{...sampleKey,name:"work",model_mode:"all",budget_limit_minor:1250000,expires_at:"2030-01-01T00:00:00Z"}}:{items:[]}}));vi.stubGlobal("fetch",fetchMock);
    render(withZh(<KeysPanel/>));fireEvent.click(screen.getByRole("button",{name:"创建 API Key"}));fireEvent.change(screen.getByLabelText("密钥名称"),{target:{value:"work"}});fireEvent.click(screen.getByLabelText("设置累计总上限"));fireEvent.change(screen.getByLabelText("USD"),{target:{value:"1.25"}});
    fireEvent.click(screen.getByLabelText("指定到期时间"));fireEvent.change(screen.getByLabelText(/当地时间/),{target:{value:"2030-01-01T00:00"}});fireEvent.click(screen.getByRole("button",{name:"创建"}));
    await waitFor(()=>expect(screen.getByTestId("key-secret")).toBeTruthy());
    const posts=fetchMock.mock.calls.filter(([,init])=>init?.method==="POST");expect(posts).toHaveLength(1);expect(JSON.parse(String(posts[0][1]?.body))).toMatchObject({name:"work",model_mode:"all",allowlist:[],budget_limit_minor:1250000});
    expect(screen.getByRole("link",{name:"Agent 配置"}).getAttribute("href")).toContain("key_id=key_1");expect(window.location.href).not.toContain(sampleKey.key);expect(screen.queryByRole("button",{name:"发送站内测试请求"})).toBeNull();
  });

});
function deferredJson() {
  let resolve!: (value: { ok: boolean; json: () => Promise<unknown> }) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<{ ok: boolean; json: () => Promise<unknown> }>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return {
    promise,
    resolve(ok: boolean, body: unknown) {
      resolve({ ok, json: async () => body });
    },
    reject(reason?: unknown) {
      reject(reason);
    },
  };
}
