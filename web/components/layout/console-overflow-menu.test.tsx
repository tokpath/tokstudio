/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ConsoleOverflowMenu } from "./console-overflow-menu";
import { withZh } from "@/lib/test-i18n";

vi.mock("next-themes", () => ({
  useTheme: () => ({ theme: "system", setTheme: vi.fn(), resolvedTheme: "light" }),
}));

describe("ConsoleOverflowMenu", () => {
  afterEach(() => {
    cleanup();
  });

  it("keeps search language theme and notifications inside the extra menu", () => {
    const onCommand = vi.fn();
    render(withZh(<ConsoleOverflowMenu onCommand={onCommand} showBell />));
    fireEvent.click(screen.getByTestId("chrome-overflow-trigger"));
    expect(screen.getByTestId("chrome-overflow-menu")).toBeTruthy();
    fireEvent.click(screen.getByRole("menuitem", { name: "跳转" }));
    expect(onCommand).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByTestId("chrome-overflow-trigger"));
    expect(screen.getByRole("menuitem", { name: "通知" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "语言" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "主题" })).toBeTruthy();
  });
});
