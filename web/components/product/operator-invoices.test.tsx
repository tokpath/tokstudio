// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { Session } from "@/lib/contracts";
import { visibleOperatorSections } from "@/lib/navigation";
import { I18nProvider } from "@/components/i18n";
import { translate } from "@/lib/i18n";
import { OperatorInvoicesScreen } from "./operator-invoices";
let permissions = ["*"];
let kind = "oem";
vi.mock("./context", () => ({ useProduct: () => ({ session: {kind:"staff",workspaces:[{operator:{id:"operator-one",kind,name:"OEM"},permissions}]} as Session }) }));
afterEach(() => { cleanup(); vi.unstubAllGlobals(); permissions=["*"];kind="oem";window.history.replaceState(null,"","/"); });
function mount() {
 const calls:string[]=[];
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>{const path=String(input);calls.push(path);return new Response(JSON.stringify({title:"",description:"",items:[{id:path.includes("procurement-invoices")?"Received invoice":"Issued invoice",values:{name:path.includes("procurement-invoices")?"Received invoice":"Issued invoice"}}],columns:[],operations:[],row_operations:[]}),{status:200});}));
 render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><OperatorInvoicesScreen operatorID="operator-one"/></QueryClientProvider>);
 return calls;
}
it("unifies both directions and clears list filters between tabs",async()=>{
 window.history.replaceState(null,"","/manage/operator-one/invoices?q=old&cursor=old-page");const calls=mount();await screen.findByText("Issued invoice");
 fireEvent.click(screen.getByRole("tab",{name:"我方收票"}));await screen.findByText("Received invoice");
 expect(calls.at(-1)).toBe("/api/v1/operators/operator-one/procurement-invoices");expect(window.location.search).toBe("?section=received");
 fireEvent.click(screen.getByRole("tab",{name:"我方开票"}));await screen.findByText("Issued invoice");expect(window.location.search).toBe("?section=issued");
});
it("platform has no upstream application even with wildcard permission",async()=>{
 kind="platform";window.history.replaceState(null,"","/manage/operator-one/invoices?section=received");const calls=mount();await screen.findByText("Issued invoice");expect(screen.queryByRole("tab",{name:"我方收票"})).toBeNull();expect(calls.every(x=>!x.includes("procurement-invoices"))).toBe(true);
});
it("procurement staff get the merged entry without issuer authority",async()=>{
 permissions=["cooperation.manage","procurement-invoices.read"];window.history.replaceState(null,"","/manage/operator-one/invoices");const calls=mount();await screen.findByText("Received invoice");expect(screen.queryByRole("tab",{name:"我方开票"})).toBeNull();expect(calls).toEqual(["/api/v1/operators/operator-one/procurement-invoices"]);
 const entries=visibleOperatorSections({operator:{kind:"oem"},permissions}).flatMap(x=>x.items.map(i=>i[0]));expect(entries).toContain("invoices");expect(entries).not.toContain("procurement-invoices");
});
it("invoice staff do not fetch procurement applications",async()=>{
 permissions=["invoices.manage","invoices.read"];window.history.replaceState(null,"","/manage/operator-one/invoices?section=received");const calls=mount();await screen.findByText("Issued invoice");expect(screen.queryByRole("tab",{name:"我方收票"})).toBeNull();expect(calls).toEqual(["/api/v1/operators/operator-one/invoices"]);
});

it.each(["zh","en","ja","ru"] as const)("renders the invoice workspace and purchase context in %s",async(locale)=>{
 window.history.replaceState(null,"","/manage/operator-one/invoices");
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>new Response(JSON.stringify(String(input).endsWith("/files") ? {
   items:[],files_available:true,details:{title:"Buyer Inc",tax_id:"TEST-1",email:"buyer@example.test"},orders:[{order_id:"order-one",name:"Package A",kind:"套餐购买",created_at:"2026-11-06T10:00:00Z",amount_money:"40"}]
 } : {title:"我方开票",description:"",items:[{id:"invoice-one",status:"requested",values:{name:"Application",applicant:"Buyer Inc",applicant_kind:"客户购买"}}],columns:[{key:"applicant",label:"申请人",kind:"text"},{key:"applicant_kind",label:"申请类型",kind:"text"}],operations:[],row_operations:[]}),{status:200})));
 render(<I18nProvider initialLocale={locale}><QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><OperatorInvoicesScreen operatorID="operator-one"/></QueryClientProvider></I18nProvider>);
 await screen.findByText("Application");
 for(const label of ["我方开票","我方收票"]) expect(screen.getByRole("tab",{name:translate(locale,label)})).toBeTruthy();
 expect(screen.getByText(translate(locale,"客户购买"))).toBeTruthy();
 fireEvent.click(screen.getByRole("button",{name:translate(locale,"查看详情"),exact:true}));
 expect(await screen.findByRole("heading",{name:translate(locale,"本次开票的购买订单")})).toBeTruthy();
 expect(screen.getByText(translate(locale,"套餐购买")+" · Package A")).toBeTruthy();
 expect(screen.getByText(translate(locale,"订单实付金额"))).toBeTruthy();
 expect(screen.getByText("buyer@example.test")).toBeTruthy();
});
