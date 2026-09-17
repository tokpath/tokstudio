/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PlaygroundClient } from "../app/console/playground/playground-client";
import { withZh } from "./test-i18n";

const echo = { id: "tokenhub/echo-1", vendor: "tokenhub", display_name: "Echo" };

describe("PlaygroundClient", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("shows an empty catalog instead of a placeholder model", () => {
    render(withZh(<PlaygroundClient models={[]} />));
    expect(screen.getByText("暂无可用模型")).toBeTruthy();
    expect(screen.queryByLabelText("试用模型")).toBeNull();
  });

  it("does not treat a catalog load failure as no models", () => {
    render(withZh(<PlaygroundClient models={[]} catalogOk={false} catalogMessage="catalog down" />));
    expect(screen.getByText("加载失败")).toBeTruthy();
    expect(screen.getByText("catalog down")).toBeTruthy();
    expect(screen.queryByText("暂无可用模型")).toBeNull();
  });

  it("stops on a URL model that is not in the catalog", () => {
    render(withZh(<PlaygroundClient models={[echo]} initialModel="missing/model" />));
    expect(screen.getByTestId("model-entry-error").getAttribute("data-reason")).toBe("missing");
    expect(screen.getByText("目录里没有这个模型")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "发送" })).toBeNull();
  });

  it("keeps the selected model from the catalog query", () => {
    render(
      withZh(
        <PlaygroundClient
          models={[echo, { id: "google/gemini-flash", vendor: "google", display_name: "Gemini Flash" }]}
          initialModel="google/gemini-flash"
        />,
      ),
    );
    expect(screen.getByLabelText("试用模型")).toHaveProperty("value", "google/gemini-flash");
  });

  it("posts chat completions with the catalog model", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ choices: [{ message: { content: "ok" } }] }),
    });
    vi.stubGlobal("fetch", fetchMock);
    render(withZh(<PlaygroundClient models={[echo]} initialModel="tokenhub/echo-1" />));
    fireEvent.change(screen.getByLabelText("试用消息"), { target: { value: "hello" } });
    fireEvent.click(screen.getByRole("button", { name: "发送" }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    const [url, init] = fetchMock.mock.calls[0] as [string, { body: string }];
    expect(String(url)).toContain("/v1/chat/completions");
    expect(JSON.parse(init.body).model).toBe("tokenhub/echo-1");
  });

  it("does not send a non-chat catalog model through chat completions", () => {
    render(
      withZh(
        <PlaygroundClient
          models={[
            {
              id: "bytedance/seedream",
              vendor: "bytedance",
              display_name: "Seedream",
              kind: "image",
              capabilities: { supported_endpoints: ["/v1/images/generations"] },
            },
          ]}
          initialModel="bytedance/seedream"
        />,
      ),
    );
    expect(screen.getByTestId("model-entry-error").getAttribute("data-reason")).toBe("notChat");
    expect(screen.queryByRole("button", { name: "发送" })).toBeNull();
  });

  it("keeps the prompt and earlier replies when a later request fails", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ choices: [{ message: { content: "first-ok" } }] }),
      })
      .mockResolvedValueOnce({
        ok: false,
        status: 429,
        json: async () => ({ error: { message: "超过并发限额" } }),
      });
    vi.stubGlobal("fetch", fetchMock);
    render(withZh(<PlaygroundClient models={[echo]} initialModel="tokenhub/echo-1" />));
    fireEvent.change(screen.getByLabelText("试用消息"), { target: { value: "hello" } });
    fireEvent.click(screen.getByRole("button", { name: "发送" }));
    await waitFor(() => expect(screen.getByText("first-ok")).toBeTruthy());
    fireEvent.change(screen.getByLabelText("试用消息"), { target: { value: "again" } });
    fireEvent.click(screen.getByRole("button", { name: "继续提问" }));
    await waitFor(() => expect(screen.getAllByText("超过并发限额").length).toBeGreaterThan(0));
    expect(screen.getByText("first-ok")).toBeTruthy();
    expect(screen.getByLabelText("试用消息")).toHaveProperty("value", "again");
  });
});
