/** @vitest-environment jsdom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi, afterEach } from "vitest";
import { StartUsingLink } from "../components/start-using-link";
import { Button } from "../components/ui/button";
import { loginHref } from "./login-next";
import { useModelHref } from "./model-use";

describe("StartUsingLink", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("keeps guests on login with model instructions as next", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: false,
        status: 403,
        json: async () => ({ error: { message: "未授权" } }),
      }),
    );
    render(
      <StartUsingLink model={{ id: "tokenhub/echo-1", kind: "text" }}>开始使用</StartUsingLink>,
    );
    const link = screen.getByRole("link", { name: "开始使用" });
    expect(link.getAttribute("href")).toBe(loginHref(useModelHref({id:"tokenhub/echo-1",kind:"text"})));
    await waitFor(() => {
      expect(link.getAttribute("href")).toBe(loginHref(useModelHref({id:"tokenhub/echo-1",kind:"text"})));
    });
  });

  it("sends signed-in users to model instructions", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ user: { roles: ["end_user"] } }),
      }),
    );
    render(
      <StartUsingLink model={{ id: "tokenhub/echo-1", kind: "text" }}>开始使用</StartUsingLink>,
    );
    await waitFor(() => {
      expect(screen.getByRole("link", { name: "开始使用" }).getAttribute("href")).toBe(
        "/app/docs?model=tokenhub%2Fecho-1&tab=agent",
      );
    });
  });

  it("keeps the filled button styles when used with Button asChild", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ user: { roles: ["end_user"] } }),
      }),
    );
    render(
      <Button asChild>
        <StartUsingLink model={{ id: "tokenhub/echo-1", kind: "text" }}>开始使用</StartUsingLink>
      </Button>,
    );
    expect(screen.getByRole("link", { name: "开始使用" }).className).toContain("bg-brand");
  });
});
