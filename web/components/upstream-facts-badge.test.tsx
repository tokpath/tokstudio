/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { UpstreamFactsBadge } from "./upstream-facts-badge";

afterEach(() => {
  cleanup();
});

describe("UpstreamFactsBadge", () => {
  it("shows the locked missing copy and never a green check", () => {
    render(<UpstreamFactsBadge facts={{ request_id: "req_gap" }} />);
    const badge = screen.getByTestId("upstream-facts-badge");
    expect(badge.textContent).toBe("缺上游元数据");
    expect(badge.getAttribute("data-complete")).toBe("false");
    expect(badge.className).not.toContain("success");
    expect(badge.className).toContain("--muted");
    expect(screen.queryByText("openai")).toBeNull();
    expect(screen.queryByText("gpt-4")).toBeNull();
  });

  it("opens a read-only JSON drawer on hover when facts are complete", () => {
    render(
      <UpstreamFactsBadge
        facts={{ provider: "prd_echo", model: "echo-up", request_id: "req_ok", fact_source: "sandbox" }}
      />,
    );
    const badge = screen.getByTestId("upstream-facts-badge");
    expect(badge.getAttribute("data-complete")).toBe("true");
    expect(badge.textContent).toContain("prd_echo");
    fireEvent.mouseEnter(badge.parentElement as HTMLElement);
    const drawer = screen.getByTestId("upstream-facts-drawer");
    expect(drawer.textContent).toContain("prd_echo");
    expect(drawer.textContent).toContain("sandbox");
    expect(drawer.textContent).not.toMatch(/estimate|invented/i);
  });
});
