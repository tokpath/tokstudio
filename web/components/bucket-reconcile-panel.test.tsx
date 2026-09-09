/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { BucketReconcilePanel } from "./bucket-reconcile-panel";
import { withZh } from "@/lib/test-i18n";
import type { ReconcileView } from "@/lib/bucket-reconcile";

afterEach(() => {
  cleanup();
});

const stub: ReconcileView = {
  buckets: { available_minor: 9_000_000, reserved_minor: 1_000_000, withdrawable_minor: 0 },
  usage_totals: { requests: 2, customer_minor: 160000, charge_minor: 160000, pending_count: 1 },
  items: [
    {
      request_id: "req_ok",
      usage_id: "usg_ok",
      occurred_at: "2026-09-09T00:00:00Z",
      public_model_id: "tokenhub/echo-1",
      match: true,
      status: "match",
      usage_minor: 160000,
      charge_minor: 160000,
      reserved_minor: 0,
    },
    {
      request_id: "req_gap",
      usage_id: "usg_gap",
      occurred_at: "2026-09-09T00:01:00Z",
      public_model_id: "tokenhub/echo-1",
      match: false,
      status: "mismatch",
      already_pending: true,
      missing_usage: true,
      usage_minor: 0,
      charge_minor: 0,
      reserved_minor: 1_000_000,
    },
  ],
  pending: [{ request_id: "req_gap", usage_id: "usg_gap", status: "pending_reconciliation" }],
};

describe("BucketReconcilePanel", () => {
  it("renders match as green OK and mismatch as danger with only flag-pending", () => {
    render(withZh(<BucketReconcilePanel scope="user" initial={stub} />));

    expect(screen.getByText("余额")).toBeTruthy();
    expect(screen.getByText("冻结")).toBeTruthy();
    expect(screen.getByText("可提现")).toBeTruthy();
    expect(screen.getAllByTestId("diff-match").length).toBeGreaterThan(0);
    expect(screen.getAllByTestId("diff-mismatch").length).toBeGreaterThan(0);
    expect(screen.getByRole("heading", { name: "待对账队列" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "送入待对账队列" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /估扣|估算扣款|estimate/i })).toBeNull();
    expect(screen.queryByText("估扣")).toBeNull();
    expect(screen.queryByText("估算扣款")).toBeNull();
  });

  it("shows an honest empty usage state without charts", () => {
    render(
      withZh(
        <BucketReconcilePanel
          scope="channel"
          initial={{ buckets: { available_minor: 0, reserved_minor: 0, withdrawable_minor: 0 }, items: [], pending: [] }}
        />,
      ),
    );
    expect(screen.getByText("暂无 usage")).toBeTruthy();
    expect(screen.queryByTestId("usage-trend-chart")).toBeNull();
    expect(screen.queryByRole("button", { name: /估扣|estimate/i })).toBeNull();
  });
});
