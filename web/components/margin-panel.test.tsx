/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";

vi.mock("next/link", () => ({
  default: ({ href, children }: { href: string; children: ReactNode }) => <a href={href}>{children}</a>,
}));

vi.mock("@tanstack/react-query", () => ({
  useQuery: () => ({
    data: {
      item: { attempt_cost_minor: 40, sell_minor: 20, margin_minor: -20, pending_count: 1, items: [] },
      items: [
        {
          attempt_id: "atm_neg",
          request_id: "req_neg",
          cost_source: "TokenHub",
          cost_minor: 40,
          sell_minor: 20,
          margin_minor: -20,
          prices: { upstream: "0.1/0.2", wholesale: "0.3/0.4", sell: "0.5/0.6" },
        },
      ],
    },
    refetch: async () => undefined,
  }),
}));

vi.mock("@/lib/client", () => ({
  apiClient: async () => ({ items: [] }),
}));

vi.mock("../app/admin/list-panel", () => ({
  AdminListPanel: ({
    emptyTitle,
    title,
    columns,
  }: {
    emptyTitle: string;
    title: string;
    columns: Array<{ id?: string; header?: string; cell?: (ctx: { row: { original: Record<string, unknown> } }) => ReactNode }>;
  }) => {
    const row = {
      attempt_id: "atm_neg",
      request_id: "req_neg",
      cost_minor: 40,
      sell_minor: 20,
      margin_minor: -20,
      prices: { upstream: "0.1/0.2", wholesale: "0.3/0.4", sell: "0.5/0.6" },
    };
    return (
      <section>
        <h2>{title}</h2>
        <p>{emptyTitle}</p>
        <table>
          <thead>
            <tr>
              {columns.map((col) => (
                <th key={col.id || String(col.header)}>{col.header}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            <tr>
              {columns.map((col) => (
                <td key={col.id || String(col.header)}>
                  {col.cell ? col.cell({ row: { original: row } }) : null}
                </td>
              ))}
            </tr>
          </tbody>
        </table>
      </section>
    );
  },
}));

import AdminMarginPage from "../app/admin/margin/page";
import { withZh } from "@/lib/test-i18n";

afterEach(() => {
  cleanup();
});

describe("admin margin page", () => {
  it("shows TokenHub source, empty copy lock, negative danger, and sealed tickets only", () => {
    render(withZh(<AdminMarginPage />));
    expect(screen.getAllByRole("heading", { name: "成本/毛利" }).length).toBeGreaterThan(0);
    expect(screen.getByText("TokenHub")).toBeTruthy();
    expect(screen.getByText("暂无 attempt 成本")).toBeTruthy();
    expect(screen.getByTestId("margin-danger").className).toContain("--danger");
    expect(screen.getByRole("button", { name: "补成本" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "调毛利" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /估扣|估算扣款|estimate/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /智能路由|smart routing/i })).toBeNull();
    expect(screen.queryByLabelText(/手填成本|estimate cost/i)).toBeNull();
  });
});
