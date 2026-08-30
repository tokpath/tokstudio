/** @vitest-environment jsdom */
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ConfirmButton } from "./confirm-button";

describe("ConfirmButton", () => {
  it("keeps the trigger label and confirms in a dialog", async () => {
    const onConfirm = vi.fn();
    render(
      <ConfirmButton title="确认调整额度" description="发放和扣减都会写审计。" onConfirm={onConfirm}>
        调整额度
      </ConfirmButton>,
    );

    expect(screen.getByRole("button", { name: "调整额度" })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "确认调整额度" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "调整额度" }));
    expect(screen.getByRole("heading", { name: "确认调整额度" })).toBeTruthy();
    expect(screen.getByText("发放和扣减都会写审计。")).toBeTruthy();
    expect(screen.getByRole("button", { name: "确认" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "取消" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "确认" }));
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
  });

  it("does not open the dialog when validate fails", () => {
    const onConfirm = vi.fn();
    render(
      <ConfirmButton title="确认创建渠道" validate={() => false} onConfirm={onConfirm}>
        创建渠道
      </ConfirmButton>,
    );
    fireEvent.click(screen.getByRole("button", { name: "创建渠道" }));
    expect(screen.queryByRole("heading", { name: "确认创建渠道" })).toBeNull();
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
