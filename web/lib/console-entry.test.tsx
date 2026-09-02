/** @vitest-environment jsdom */
import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { ConsoleEntryButton } from "../components/layout/console-entry";
import { withZh } from "./test-i18n";
import { loginHref } from "./login-next";
import { CONSOLE_ENTRY_PATH } from "./console-home";

describe("ConsoleEntryButton", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: false,
        status: 403,
        json: async () => ({ error: { message: "未授权" } }),
      }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("starts as the console entry and then points guests at login", async () => {
    render(withZh(<ConsoleEntryButton />));
    const link = screen.getByRole("link", { name: "控制台" });
    expect(link.getAttribute("href")).toBe(CONSOLE_ENTRY_PATH);
    await waitFor(() => {
      expect(link.getAttribute("href")).toBe(loginHref(CONSOLE_ENTRY_PATH));
    });
  });
});
