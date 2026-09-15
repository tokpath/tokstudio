/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import SettingsPanel from "@/app/console/settings-panel";
import { withZh } from "@/lib/test-i18n";

function submitPassword() {
  const submit = screen.getAllByRole("button", { name: "修改密码" }).find((button) => button.getAttribute("type") === "submit");
  fireEvent.click(submit!);
}

describe("SettingsPanel password dialog", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.includes("/v1/me/password") && init?.method === "POST") {
          return {
            ok: false,
            json: async () => ({ error: { message: "当前密码不正确" } }),
          };
        }
        return {
          ok: true,
          json: async () => ({ user: { email: "user@tokenhub.local", display_name: "Ada", locale: "zh", channel_org_id: "chn_a" } }),
        };
      }),
    );
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("keeps the dialog open and shows the API error next to submit", async () => {
    render(withZh(<SettingsPanel />));
    await screen.findByText(/user@tokenhub.local/);
    fireEvent.click(screen.getByRole("button", { name: "修改密码" }));
    fireEvent.change(screen.getByLabelText("当前密码"), { target: { value: "old-password" } });
    fireEvent.change(screen.getByLabelText("新密码"), { target: { value: "new-password" } });
    submitPassword();
    await waitFor(() => {
      expect(screen.getByTestId("submit-status").textContent).toContain("当前密码不正确");
    });
    expect(screen.getByRole("heading", { name: "修改密码" })).toBeTruthy();
    expect(screen.getByLabelText("当前密码")).toHaveProperty("value", "old-password");
  });

  it("shows a network error in the dialog instead of failing silently", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        if (String(input).includes("/v1/me/password") && init?.method === "POST") {
          throw new TypeError("Failed to fetch");
        }
        return {
          ok: true,
          json: async () => ({ user: { email: "user@tokenhub.local", display_name: "Ada", locale: "zh" } }),
        };
      }),
    );
    render(withZh(<SettingsPanel />));
    await screen.findByText(/user@tokenhub.local/);
    fireEvent.click(screen.getByRole("button", { name: "修改密码" }));
    fireEvent.change(screen.getByLabelText("当前密码"), { target: { value: "old-password" } });
    fireEvent.change(screen.getByLabelText("新密码"), { target: { value: "new-password" } });
    submitPassword();
    await waitFor(() => {
      expect(screen.getByTestId("submit-status").textContent).toContain("网络不可用，请重试。");
    });
  });
});
