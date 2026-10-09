/** @vitest-environment jsdom */
import { cleanup,render,screen,fireEvent,within } from "@testing-library/react";
import { afterEach,it,expect,vi } from "vitest";
import { QueryClient,QueryClientProvider } from "@tanstack/react-query";
import { withZh } from "@/lib/test-i18n";
import { PaymentOrderDetail } from "./order-detail";
vi.mock("next/navigation",()=>({useSearchParams:()=>new URLSearchParams()}));
vi.mock("@/components/rbac/viewer-context",()=>({useViewer:()=>({roles:["finance_admin"],userId:"finance",loading:false,signedIn:true})}));
afterEach(()=>{cleanup();vi.unstubAllGlobals()});
const order={id:"pay_one",user_id:"u1",channel_org_id:"channel",payee_channel_org_id:"owner",adapter:"manual",purpose:"wallet",status:"paid",amount_minor:10000,credit_minor:2000000,currency:"CNY",created_at:"2026-10-10",fulfilled_at:"2026-10-10"};
const detail={item:{order,allocations:[],events:[]},customer:{display_name:"Alice",email:"alice@example.test"},channel_codes:{channel:"Referral channel",owner:"OEM brand"}};
function page(){return render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}>{withZh(<PaymentOrderDetail id="pay_one"/>)}</QueryClientProvider>)}
it("refund confirmation loads actual cash and credit impacts before allowing the mutation",async()=>{
 const fetcher=vi.fn(async(url,init)=>({ok:true,json:async()=>init?.method==="POST"?{item:{...order,status:"refunded"}}:String(url).endsWith("refund-preview")?{item:{order,can_refund:true,credit_reclaim_minor:2000000}}:detail}));vi.stubGlobal("fetch",fetcher);page();fireEvent.click(await screen.findByRole("button",{name:"登记线下退款"}));const dialog=await screen.findByRole("dialog");expect(within(dialog).getByText(/回收充值额度 \$2.00 USD.*¥100.00 CNY/)).toBeTruthy();expect(fetcher.mock.calls.some(([,init])=>init?.method==="POST")).toBe(false);fireEvent.click(within(dialog).getByRole("button",{name:"确认"}));await screen.findByText(/订单 pay_one 已更新/);const call=fetcher.mock.calls.find(([,init])=>init?.method==="POST")!;expect(JSON.parse(call[1].body).occurred_at).toBeTruthy();
});
it("a failed preview cannot open a refund confirmation or show zero as an impact",async()=>{
 const fetcher=vi.fn(async(url)=>({ok:!String(url).endsWith("refund-preview"),json:async()=>String(url).endsWith("refund-preview")?{error:{message:"账务暂不可读"}}:detail}));vi.stubGlobal("fetch",fetcher);page();fireEvent.click(await screen.findByRole("button",{name:"登记线下退款"}));await screen.findByText("账务暂不可读");expect(screen.queryByRole("dialog")).toBeNull();
});
