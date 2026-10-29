// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { Entity, ResourcePage } from "@/lib/contracts";
import { ModelPricingCard } from "./pricing-cards";
import { pricingRefetchInterval } from "./pricing-refresh";

afterEach(cleanup);
const pending = (enabled: boolean, needs_attention = false) => ({ kind: "listing", enabled, needs_attention, status: "announced", effective_at: "2030-01-01T00:00:00Z" });
function show(row: Entity) { return render(<ModelPricingCard row={row} canPrice={false} canTemplate={false} onPrice={() => {}} onTemplate={() => {}} />); }
it("distinguishes a scheduled listing from a model that has never been listed", () => {
 show({ id: "GPT-6.1 Sol", status: "not_listed", values: { pending_changes: [pending(true)] } });
 expect(screen.getByText("上架待生效")).toBeTruthy();
 expect(screen.queryByText("尚未上架，可先准备计费与价格")).toBeNull();
 expect(screen.getByText("预计生效时间", { exact: false })).toBeTruthy();
 expect(screen.getByText("生效后列表自动更新", { exact: false })).toBeTruthy();
});
it("keeps an active listing available while unlisting is pending", () => {
 show({ id: "GPT-6.1 Sol", status: "available", values: { pending_changes: [pending(false)] } });
 expect(screen.getByText("下架待生效")).toBeTruthy();
 expect(screen.queryByText("尚未上架，可先准备计费与价格")).toBeNull();
});
it("does not promise an activation time for a change needing attention", () => {
 show({ id: "GPT-6.1 Sol", status: "not_listed", values: { pending_changes: [pending(true, true)] } });
 expect(screen.getByText("上架变更待处理")).toBeTruthy();
 expect(screen.queryByText("预计生效时间", { exact: false })).toBeNull();
});
it("refreshes an unfinished publication and stops after activation or a review blocker", () => {
 const page = (row: Entity): ResourcePage => ({ title: "模型与定价", description: "", items: [row], columns: [], operations: [], row_operations: [] });
 expect(pricingRefetchInterval(page({ id: "m", status: "not_listed", values: { pending_changes: [pending(true)] } }))).toBe(3000);
 expect(pricingRefetchInterval(page({ id: "m", status: "available", values: { pending_changes: [] } }))).toBe(false);
 expect(pricingRefetchInterval(page({ id: "m", status: "not_listed", values: { pending_changes: [pending(true, true)] } }))).toBe(false);
});
