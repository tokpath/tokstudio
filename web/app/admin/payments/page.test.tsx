/** @vitest-environment jsdom */
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { QueryClient,QueryClientProvider } from "@tanstack/react-query";
import { withZh } from "@/lib/test-i18n";
import { PaymentOrdersPanel } from "./orders-panel";
vi.mock("next/navigation",()=>({useRouter:()=>({push:vi.fn()}),usePathname:()=>"/admin/payments",useSearchParams:()=>new URLSearchParams()}));
vi.mock("@/components/rbac/viewer-context",()=>({useViewer:()=>({signedIn:true,loading:false,roles:["audit_readonly"],userId:"auditor"})}));
afterEach(()=>{cleanup();window.history.replaceState(null,"","/admin/payments");vi.unstubAllGlobals()});
const order={id:"pay_one",user_id:"usr_one",user_email:"alice@example.test",user_name:"Alice",channel_code:"direct",adapter:"alipay",purpose:"wallet",status:"paid",currency:"CNY",amount_minor:10000,credit_minor:13986013,created_at:"2026-09-18T00:00:00Z",fulfilled_at:"2026-09-18"};
function page(){return render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}>{withZh(<PaymentOrdersPanel/>)}</QueryClientProvider>)}
it("an auditor can read original order details and human amounts without cash actions",async()=>{
 vi.stubGlobal("fetch",vi.fn(async()=>({ok:true,json:async()=>({items:[order]})})));page();await screen.findByText("alice@example.test");expect(screen.getByText("¥100.00 CNY")).toBeTruthy();expect(screen.getByText(/13.99 USD/)).toBeTruthy();expect(screen.getByRole("link",{name:"pay_one"}).getAttribute("href")).toContain("/admin/payments/pay_one");expect(screen.queryByRole("button",{name:"退款"})).toBeNull();expect(screen.queryByRole("button",{name:"线下收款划拨"})).toBeNull();
});
it("a next cursor fetches a new server page instead of filtering the first page",async()=>{
 const fetcher=vi.fn(async(url)=>({ok:true,json:async()=>String(url).includes("cursor=pay_one")?{items:[{...order,id:"pay_later"}],next_cursor:""}:{items:[order],next_cursor:"pay_one"}}));vi.stubGlobal("fetch",fetcher);page();fireEvent.click(await screen.findByRole("button",{name:"下一页"}));await screen.findByRole("link",{name:"pay_later"});expect(fetcher.mock.calls.some(([url])=>String(url).includes("cursor=pay_one"))).toBe(true);
});
