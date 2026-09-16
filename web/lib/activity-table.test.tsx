/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ActivityTable } from "../app/console/activity/activity-table";
import { withZh } from "./test-i18n";

describe("ActivityTable", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          items: [
            {
              id: "usg_1",
              request_id: "req_tiny",
              state: "confirmed",
              customer_amount_minor: 26,
              public_model_id: "tokenhub/echo-1",
              occurred_at: "2026-09-16T00:00:00.000Z",
            },
            {
              id: "usg_2",
              request_id: "req_mystery",
              state: "weird_internal_code",
              customer_amount_minor: 1_000_000,
              public_model_id: "tokenhub/echo-1",
            },
          ],
        }),
      }),
    );
  });

  it("shows sub-cent fees and does not paint unknown status as success", async () => {
    render(withZh(<ActivityTable />));
    await waitFor(() => expect(screen.getByText("小于 $0.01")).toBeTruthy());
    expect(screen.getByText("$1.00")).toBeTruthy();
    expect(screen.getByText("已确认")).toBeTruthy();
    expect(screen.getByText("未知状态（weird_internal_code）")).toBeTruthy();
    for (const node of screen.getAllByText("详情")) {
      fireEvent.click(node);
    }
    expect(screen.getAllByText("请求编号").length).toBeGreaterThan(0);
    expect(screen.getByText("req_tiny")).toBeTruthy();
  });
});
