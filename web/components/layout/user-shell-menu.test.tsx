/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { withZh } from "@/lib/test-i18n";
import { UserShellBell, UserShellRightZone } from "./user-shell-menu";

const push = vi.fn();
const refresh = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, refresh }),
}));

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

  it("shows the nailed available field and real profile, never a fake key list", async () => {
    render(withZh(<UserShellRightZone />));
    await waitFor(() => expect(screen.getByTestId("balance-pill").textContent).toBe("$12.50"));
    expect(screen.getByTestId("balance-pill").getAttribute("data-field")).toBe("available");
    expect(screen.getByTestId("balance-pill").getAttribute("href")).toBe("/app/wallet");
    expect(screen.queryByText("$0.00")).toBeNull();
    fireEvent.click(screen.getByTestId("avatar-trigger"));
    expect(screen.getByTestId("menu-display-name").textContent).toBe("Ada");
    expect(screen.getByTestId("menu-email").textContent).toBe("ada@example.test");
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
    expect(screen.getByText("管理员")).toBeTruthy();
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
    await waitFor(() => expect(screen.getByText("管理员")).toBeTruthy());
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
