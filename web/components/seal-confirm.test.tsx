/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SealConfirm } from "./seal-confirm";
import { withZh } from "@/lib/test-i18n";

afterEach(() => {
  cleanup();
});

describe("SealConfirm", () => {
  it("keeps the trigger label and confirms with a seal, not a lightweight dialog", async () => {
    const onConfirm = vi.fn(async () => true);
    render(
      withZh(
        <SealConfirm title="新牌价只约束之后的请求，已入账金额不会改写。" description="当前 published 会标成 superseded。" onConfirm={onConfirm}>
          发布价格
        </SealConfirm>,
      ),
    );

    expect(screen.getByRole("button", { name: "发布价格" })).toBeTruthy();
    expect(screen.queryByText("SEAL")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "发布价格" }));
    expect(screen.getByText("SEAL")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "新牌价只约束之后的请求，已入账金额不会改写。" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "盖章确认" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "取消" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "确认" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "盖章确认" }));
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.queryByText("SEAL")).toBeNull());
  });

  it("does not open the seal when validate fails", () => {
    const onConfirm = vi.fn();
    render(
      withZh(
        <SealConfirm title="新牌价只约束之后的请求，已入账金额不会改写。" validate={() => false} onConfirm={onConfirm}>
          发布价格
        </SealConfirm>,
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "发布价格" }));
    expect(screen.queryByText("SEAL")).toBeNull();
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
