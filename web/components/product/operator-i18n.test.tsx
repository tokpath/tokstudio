// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { I18nProvider, LanguageControl } from "@/components/i18n";
import { translate } from "@/lib/i18n";
import type { ResourcePage, Session } from "@/lib/contracts";
import { ReadFacts, recordTitle, ResourceCollection, ResourceScreen } from "./operator-resources";
import { TakeoversScreen } from "./takeovers";
const session: Session = { kind: "staff", staff: { id: "staff-1", email: "staff@example.test", display_name: "负责人" }, workspaces: [{ operator: { id: "operator-one", name: "山海服务", kind: "oem", brand_id: "brand-one" }, permissions: ["*"] }] };
vi.mock("./context", () => ({ useProduct: () => ({ session }) }));
afterEach(() => { cleanup(); vi.unstubAllGlobals(); document.cookie = "NEXT_LOCALE=; Max-Age=0; Path=/"; document.documentElement.lang = "zh-CN"; });
function show(children: React.ReactNode) { return render(<I18nProvider initialLocale="en"><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><LanguageControl />{children}</QueryClientProvider></I18nProvider>); }
function response(value: unknown) { return new Response(JSON.stringify(value), { status: 200, headers: { "content-type": "application/json" } }); }
it("sends localized search and status filters to the server without trimming authorized results", async () => {
 const page: ResourcePage = { search_supported: true, status_supported: true, title: "退款事项", description: "", items: [{ id: "refund-case", presentation:{title:"refund-case",labels:{}}, status: "completed", values: { record_kind: "case" } }, { id: "refund-leg", presentation:{title:"refund-leg",labels:{}}, status: "pending", values: { record_kind: "leg" } }], columns: [{ key: "record_kind", label: "记录类型", kind: "text" }], operations: [], row_operations: [] };
 const fetch = vi.fn(async () => response(page)); vi.stubGlobal("fetch", fetch); show(<ResourceScreen operatorID="operator-one" resource="refunds" />);
 await screen.findByText("refund-case");
 fireEvent.change(screen.getByRole("textbox"), { target: { value: "Refund component" } }); fireEvent.click(screen.getByRole("button", { name: "Search", exact: true }));
 await waitFor(() => expect(String(fetch.mock.calls.at(-1)?.[0])).toContain("q=Refund+component"));
 // The server may include the case and its linked components. Preserve them all.
 await screen.findByText("refund-case"); expect(screen.getByText("refund-leg")).toBeTruthy();
 fireEvent.change(screen.getByTestId("language-selector"), { target: { value: "ja" } });
 fireEvent.change(screen.getByRole("combobox", { name: translate("ja", "筛选状态") }), { target: { value: "completed" } });
 await waitFor(() => expect(String(fetch.mock.calls.at(-1)?.[0])).toContain("status=completed"));
 await screen.findByText("refund-case"); expect(screen.getByText("refund-leg")).toBeTruthy();
 expect(screen.getByRole("option", { name: translate("ja", "已完成") })).toBeTruthy();
});
it("translates metadata while preserving names and evidence matching system copy", () => {
 const row = { id: "customer-a", values: { name: "客户", evidence: "查看详情", kind: "package" } }; expect(recordTitle(row, "customers", "ru")).toBe("客户");
 show(<><ResourceCollection embedded operatorID="operator-one" resource="customers" layout="cards" page={{ title: "", description: "", items: [row], columns: [], operations: [], row_operations: [] }} /><ReadFacts value={{ BuyerID: "buyer-a", PaidMoney: "1.000000000000000001", evidence: "查看详情" }} /></>);
 expect(screen.getByRole("heading", { name: "客户" })).toBeTruthy(); expect(screen.getByText("Buyer ID")).toBeTruthy(); expect(screen.getByText("Original sale value settlement currency")).toBeTruthy(); expect(screen.getByText("1.000000000000000001")).toBeTruthy(); expect(screen.getByText("查看详情")).toBeTruthy();
 fireEvent.click(screen.getByRole("button", { name: "Customer workspace" })); expect(screen.getByRole("dialog")).toBeTruthy(); expect(screen.getByRole("heading", { name: "客户" })).toBeTruthy();
});
it("shows simplified takeover copy in all supported languages", async()=>{
 vi.stubGlobal("fetch",vi.fn(async()=>response({items:[],channels:[{id:"channel",name:"Channel",legal_name:"",lifecycle:"active"}]})));
 show(<TakeoversScreen operatorID="operator-one"/>);
 await screen.findByRole("button",{name:"Take over channel"});
 for(const locale of ["zh","en","ja","ru","hi","ms","th"] as const){
  fireEvent.change(screen.getByTestId("language-selector"),{target:{value:locale}});
  expect(screen.getByText(translate(locale,"查看停业告知、渠道接管进度和结果。采购余额不退，双方资金交接在线下处理。"))).toBeTruthy();
 }
});
it("localizes the accounting explanation without treating pending costs as zero", () => {
 const note = "Fulfillment cost is consumed weighted procurement cost, or verified supplier cost for the platform. Gift cost is a subset and is not deducted twice. Pending costs are unknown, not zero."; for (const locale of ["zh", "ja", "ru"] as const) expect(translate(locale, note)).not.toBe(note); expect(translate("zh", note)).toContain("未决成本为未知，并非零");
});
