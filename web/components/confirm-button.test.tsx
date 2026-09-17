/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ConfirmButton } from "./confirm-button";
import { withZh } from "@/lib/test-i18n";

afterEach(() => {
  cleanup();
});

describe("ConfirmButton", () => {
  it("keeps the trigger label and closes only when onConfirm returns true", async () => {
    const onConfirm = vi.fn(async () => true);
    render(
      withZh(
        <ConfirmButton title="确认调整额度" description="发放和扣减都会写审计。" onConfirm={onConfirm}>
          调整额度
        </ConfirmButton>,
      ),
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
    await waitFor(() => expect(screen.queryByRole("heading", { name: "确认调整额度" })).toBeNull());
  });

  it("does not open the dialog when validate fails", () => {
    const onConfirm = vi.fn();
    render(
      withZh(
        <ConfirmButton title="确认创建渠道" validate={() => false} onConfirm={onConfirm}>
          创建渠道
        </ConfirmButton>,
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "创建渠道" }));
    expect(screen.queryByRole("heading", { name: "确认创建渠道" })).toBeNull();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("does not treat a void return as success", async () => {
    const onConfirm = vi.fn(() => {
      return;
    });
    render(
      withZh(
        <ConfirmButton title="确认调整额度" onConfirm={onConfirm}>
          调整额度
        </ConfirmButton>,
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "调整额度" }));
    fireEvent.click(screen.getByRole("button", { name: "确认" }));
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(screen.getByRole("heading", { name: "确认调整额度" })).toBeTruthy();
  });

  it("stays open when onConfirm returns false", async () => {
    const onConfirm = vi.fn(async () => false);
    render(
      withZh(
        <ConfirmButton title="确认调整额度" onConfirm={onConfirm}>
          调整额度
        </ConfirmButton>,
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "调整额度" }));
    fireEvent.click(screen.getByRole("button", { name: "确认" }));
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(screen.getByRole("heading", { name: "确认调整额度" })).toBeTruthy();
  });

  it("stays open when onConfirm throws", async () => {
    const onConfirm = vi.fn(async () => {
      throw new TypeError("Failed to fetch");
    });
    render(
      withZh(
        <ConfirmButton title="确认调整额度" onConfirm={onConfirm}>
          调整额度
        </ConfirmButton>,
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "调整额度" }));
    fireEvent.click(screen.getByRole("button", { name: "确认" }));
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(screen.getByRole("heading", { name: "确认调整额度" })).toBeTruthy();
  });

  it("ignores a second click while the first confirm is in flight", async () => {
    let finish!: (value: boolean) => void;
    const onConfirm = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          finish = resolve;
        }),
    );
    render(
      withZh(
        <ConfirmButton title="确认调整额度" onConfirm={onConfirm}>
          调整额度
        </ConfirmButton>,
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "调整额度" }));
    fireEvent.click(screen.getByRole("button", { name: "确认" }));
    fireEvent.click(screen.getByRole("button", { name: "确认" }));
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    finish(true);
    await waitFor(() => expect(screen.queryByRole("heading", { name: "确认调整额度" })).toBeNull());
  });

  it("does not let a stale success close a reopened dialog", async () => {
    let finish!: (value: boolean) => void;
    const onConfirm = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          finish = resolve;
        }),
    );
    render(
      withZh(
        <ConfirmButton title="确认调整额度" onConfirm={onConfirm}>
          调整额度
        </ConfirmButton>,
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "调整额度" }));
    fireEvent.click(screen.getByRole("button", { name: "确认" }));
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    fireEvent.click(screen.getByRole("button", { name: "取消" }));
    await waitFor(() => expect(screen.queryByRole("heading", { name: "确认调整额度" })).toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "调整额度" }));
    expect(screen.getByRole("heading", { name: "确认调整额度" })).toBeTruthy();
    finish(true);
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(screen.getByRole("heading", { name: "确认调整额度" })).toBeTruthy();
  });
});
