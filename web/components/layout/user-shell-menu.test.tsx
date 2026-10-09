/** @vitest-environment jsdom */
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { withZh } from "@/lib/test-i18n";
import { notifyWalletChanged } from "@/lib/wallet-events";
import { UserShellBell, UserShellRightZone } from "./user-shell-menu";

const push = vi.fn();
const refresh = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, refresh }),
}));

vi.mock("@/components/rbac/viewer-context", () => ({ useViewer: () => ({ userId: "shell-user", channelType: "C" }) }));

vi.mock("@/components/brand-context",()=>({useBrand:()=>({id:"wallet-brand"})}));

function jsonResponse(ok: boolean, body: unknown, status = ok ? 200 : 503) {
  return {
    ok,
    status,
    json: async () => body,
  };
}

describe("UserShellRightZone", () => {
  beforeEach(() => {
    push.mockReset();
    refresh.mockReset();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/v1/me/balance")) {
          return jsonResponse(true, { balance: { available: "12.5", reserved: "1.00", gift_minor: 0 } });
        }
        if (url.includes("/v1/auth/logout")) {
          return jsonResponse(true, { ok: true });
        }
        if (url.includes("/v1/me")) {
          return jsonResponse(true, {
            user: {
              display_name: "Ada",
              email: "ada@example.test",
              roles: ["end_user"],
              login_methods: ["password"],
            },
          });
        }
        return jsonResponse(false, { error: { message: "missing" } });
      }),
    );
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("refreshes only the current wallet and ignores a late old balance",async()=>{
    let reads=0;let release!:(value:unknown)=>void;
    vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>{
      if(String(input).includes("/v1/me/balance")){
        reads++;if(reads===1)return await new Promise(resolve=>{release=resolve;});
        return jsonResponse(true,{balance:{available:"10"}});
      }
      return jsonResponse(true,{user:{display_name:"Ada",roles:["end_user"]}});
    }));
    render(withZh(<UserShellRightZone/>));
    await waitFor(()=>expect(reads).toBe(1));
    act(()=>notifyWalletChanged({userId:"another-user",brandId:"wallet-brand"}));
    expect(reads).toBe(1);
    act(()=>notifyWalletChanged({userId:"shell-user",brandId:"foreign-brand"}));expect(reads).toBe(1);
    act(()=>notifyWalletChanged({userId:"shell-user",brandId:"wallet-brand"}));
    await waitFor(()=>expect(screen.getByTestId("balance-pill").textContent).toBe("$10.00"));
    await act(async()=>release(jsonResponse(true,{balance:{available:"0"}})));
    expect(screen.getByTestId("balance-pill").textContent).toBe("$10.00");
  });

  it("shows the nailed available field and real profile, never a fake key list", async () => {
    render(withZh(<UserShellRightZone />));
    await waitFor(() => expect(screen.getByTestId("balance-pill").textContent).toBe("$12.50"));
    expect(screen.getByTestId("balance-pill").getAttribute("data-field")).toBe("available");
    expect(screen.getByTestId("balance-pill").getAttribute("href")).toBe("/app/wallet");
    expect(screen.queryByText("$0.00")).toBeNull();
    fireEvent.click(screen.getByTestId("avatar-trigger"));
    expect(screen.getByTestId("menu-display-name").textContent).toBe("Ada");
    expect(screen.getByTestId("menu-email").textContent).toBe("ada@example.test");
    expect(screen.getByTestId("menu-balance").textContent).toBe("$12.50");
    expect(screen.getByRole("menuitem", { name: "个人资料" }).getAttribute("href")).toBe("/app/profile");
    expect(screen.getByRole("menuitem", { name: "API 密钥" }).getAttribute("href")).toBe("/app/keys");
    expect(screen.queryByRole("menuitem", { name: "平台管理" })).toBeNull();
    expect(screen.queryByTestId("menu-platform-admin")).toBeNull();
    expect(screen.queryByText("thk_")).toBeNull();
    expect(screen.queryByText("GitHub")).toBeNull();
    expect(screen.queryByText("新手引导")).toBeNull();
  });

  it("shows — when balance fails and never writes fake $0.00", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/v1/me/balance")) {
          return jsonResponse(false, { error: { message: "boom" } });
        }
        if (url.includes("/v1/me")) {
          return jsonResponse(true, { user: { roles: ["platform_admin"] } });
        }
        return jsonResponse(false, {});
      }),
    );
    render(withZh(<UserShellRightZone />));
    await waitFor(() => expect(screen.getByTestId("balance-pill").getAttribute("data-state")).toBe("error"));
    expect(screen.getByTestId("balance-pill").textContent).toBe("—");
    expect(screen.queryByText("$0.00")).toBeNull();
    fireEvent.click(screen.getByTestId("avatar-trigger"));
    expect(screen.getByTestId("menu-display-name").textContent).toBe("—");
    expect(screen.getByTestId("menu-email").textContent).toBe("—");
    expect(screen.getByText("平台管理员")).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: "平台管理" }).getAttribute("href")).toBe("/admin");
  });

  it("shows 平台管理 only when roles include platform_admin", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/v1/me/balance")) {
          return jsonResponse(true, { balance: { available: "1" } });
        }
        if (url.includes("/v1/me")) {
          return jsonResponse(true, {
            user: { display_name: "Pat", email: "pat@example.test", roles: ["platform_admin"] },
          });
        }
        return jsonResponse(false, {});
      }),
    );
    render(withZh(<UserShellRightZone />));
    await waitFor(() => expect(screen.getByTestId("avatar-trigger")).toBeTruthy());
    fireEvent.click(screen.getByTestId("avatar-trigger"));
    const adminItem = screen.getByRole("menuitem", { name: "平台管理" });
    expect(adminItem.getAttribute("href")).toBe("/admin");
    expect(adminItem.getAttribute("aria-disabled")).toBeNull();
    expect((adminItem as HTMLElement).hasAttribute("disabled")).toBe(false);
  });

  it("does not render a disabled 平台管理 entry for finance_admin or end_user", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/v1/me/balance")) {
          return jsonResponse(true, { balance: { available: "1" } });
        }
        if (url.includes("/v1/me")) {
          return jsonResponse(true, {
            user: { display_name: "Fin", email: "fin@example.test", roles: ["finance_admin"] },
          });
        }
        return jsonResponse(false, {});
      }),
    );
    render(withZh(<UserShellRightZone />));
    await waitFor(() => expect(screen.getByText("财务")).toBeTruthy());
    fireEvent.click(screen.getByTestId("avatar-trigger"));
    expect(screen.queryByRole("menuitem", { name: "平台管理" })).toBeNull();
    expect(screen.queryByRole("link", { name: "平台管理" })).toBeNull();
    expect(screen.queryByTestId("menu-platform-admin")).toBeNull();
    expect(screen.queryAllByText("平台管理")).toHaveLength(0);
  });

  it("logs out through the real session endpoint", async () => {
    render(withZh(<UserShellRightZone />));
    await waitFor(() => expect(screen.getByTestId("avatar-trigger")).toBeTruthy());
    fireEvent.click(screen.getByTestId("avatar-trigger"));
    fireEvent.click(screen.getByRole("menuitem", { name: "退出登录" }));
    await waitFor(() => expect(push).toHaveBeenCalledWith("/login"));
    expect(fetch).toHaveBeenCalledWith("/api/v1/auth/logout", expect.objectContaining({ method: "POST" }));
  });
});

describe("UserShellBell", () => {
  it("is a grey postponed placeholder", () => {
    render(withZh(<UserShellBell />));
    const bell = screen.getByTestId("shell-bell") as HTMLButtonElement;
    expect(bell.disabled).toBe(true);
    expect(bell.getAttribute("title")).toBe("通知中心尚未开放");
  });
});

describe("UserShellRightZone admin variant", () => {
  beforeEach(() => {
    push.mockReset();
    refresh.mockReset();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/v1/me/balance")) {
          return jsonResponse(true, { balance: { available: "99" } });
        }
        if (url.includes("/v1/auth/logout")) {
          return jsonResponse(true, { ok: true });
        }
        if (url.includes("/v1/me")) {
          return jsonResponse(true, {
            user: {
              display_name: "Ops",
              email: "ops@example.test",
              roles: ["platform_admin"],
            },
          });
        }
        return jsonResponse(false, {});
      }),
    );
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("shows channel.c as an OEM administrator without a platform management link", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => jsonResponse(true, { user: { email: "channel.c@tokenhub.local", roles: ["channel_admin"] } })));
    render(withZh(<UserShellRightZone variant="admin" />));
    await screen.findByText("OEM 管理员");
    expect(screen.queryByText("用户", { exact: true })).toBeNull();
    fireEvent.click(screen.getByTestId("avatar-trigger"));
    expect(screen.queryByTestId("menu-platform-admin")).toBeNull();
    expect(screen.getByTestId("menu-email").textContent).toBe("channel.c@tokenhub.local");
  });

  it("shows avatar profile without balance or API keys, and links back to user console", async () => {
    render(withZh(<UserShellRightZone variant="admin" />));
    await waitFor(() => expect(screen.getByTestId("avatar-trigger")).toBeTruthy());
    expect(screen.queryByTestId("balance-pill")).toBeNull();
    const calledUrls = vi.mocked(fetch).mock.calls.map((call) => String(call[0]));
    expect(calledUrls.some((url) => url.includes("/v1/me/balance"))).toBe(false);
    fireEvent.click(screen.getByTestId("avatar-trigger"));
    expect(screen.getByTestId("menu-display-name").textContent).toBe("Ops");
    expect(screen.getByTestId("menu-email").textContent).toBe("ops@example.test");
    expect(screen.getByRole("menuitem", { name: "个人资料" }).getAttribute("href")).toBe("/app/profile");
    expect(screen.getByRole("menuitem", { name: "用户控制台" }).getAttribute("href")).toBe("/app");
    expect(screen.queryByRole("menuitem", { name: "API 密钥" })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: "平台管理" })).toBeNull();
    expect(screen.queryByTestId("menu-platform-admin")).toBeNull();
  });

  it("still logs out through the real session endpoint", async () => {
    render(withZh(<UserShellRightZone variant="admin" />));
    await waitFor(() => expect(screen.getByTestId("avatar-trigger")).toBeTruthy());
    fireEvent.click(screen.getByTestId("avatar-trigger"));
    fireEvent.click(screen.getByRole("menuitem", { name: "退出登录" }));
    await waitFor(() => expect(push).toHaveBeenCalledWith("/login"));
    expect(fetch).toHaveBeenCalledWith("/api/v1/auth/logout", expect.objectContaining({ method: "POST" }));
  });
});
