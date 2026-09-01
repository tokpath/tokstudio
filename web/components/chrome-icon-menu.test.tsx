/** @vitest-environment jsdom */
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ChromeIconMenu } from "./chrome-icon-menu";

describe("ChromeIconMenu", () => {
  it("keeps the trigger as an unlabeled-looking icon button and lists options in a menu", () => {
    const onChange = vi.fn();
    render(
      <ChromeIconMenu
        label="语言"
        icon={<span>🌐</span>}
        value="auto"
        options={[
          { value: "auto", label: "自动" },
          { value: "ja", label: "日本語" },
        ]}
        onChange={onChange}
      />,
    );

    expect(screen.queryByRole("menu")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "语言" }));
    expect(screen.getByRole("menu", { name: "语言" })).toBeTruthy();
    expect(screen.getByRole("menuitemradio", { name: "自动" }).getAttribute("aria-checked")).toBe("true");

    fireEvent.click(screen.getByRole("menuitemradio", { name: "日本語" }));
    expect(onChange).toHaveBeenCalledWith("ja");
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("does not fire onChange when the current value is clicked again", () => {
    const onChange = vi.fn();
    render(
      <ChromeIconMenu
        label="主题"
        icon={<span>☀</span>}
        value="system"
        options={[{ value: "system", label: "系统" }]}
        onChange={onChange}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "主题" }));
    fireEvent.click(screen.getByRole("menuitemradio", { name: "系统" }));
    expect(onChange).not.toHaveBeenCalled();
  });
});
