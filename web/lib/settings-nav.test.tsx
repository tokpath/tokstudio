/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SettingsSubnav } from "../components/console/settings-nav";
import { withZh } from "./test-i18n";

vi.mock("next/navigation", () => ({
  usePathname: () => "/app/settings",
}));

describe("SettingsSubnav", () => {
  afterEach(() => {
    cleanup();
  });

  it("keeps placeholder settings as labeled unavailable entries, not links", () => {
    render(withZh(<SettingsSubnav />));
    expect(screen.getByRole("link", { name: "账户" }).getAttribute("href")).toBe("/app/settings");
    expect(screen.queryByRole("link", { name: /团队/ })).toBeNull();
    expect(screen.getByText("团队").closest("[aria-disabled='true']")).toBeTruthy();
    expect(screen.getAllByText("未开放").length).toBeGreaterThan(0);
  });
});
