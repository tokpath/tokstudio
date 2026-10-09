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
 const fetcher=vi.fn(async(url,init)=>({ok:true,json:async()=>init?.method==="POST"?{item:{...order,status:"refunded"}}:String(url).endsWith("refund-preview")?{item:{order,can_refund:true,credit_reclaim_minor:2000000}}:detail}));vi.stubGlobal("fetch",fetcher);page();fireEvent.click(await screen.findByRole("button",{name:"登记线下退款"}));const dialog=await screen.findByRole("dialog");expect(within(dialog).getByText(/回收充值额度 \$2.00 USD.*¥100.00 CNY/)).toBeTruthy();expect(fetcher.mock.calls.some(([,init])=>init?.method==="POST")).toBe(false);fireEvent.click(within(dialog).getByRole("button",{name:"确认"}));await screen.findByText(/订单 pay_one 已退款/);const call=fetcher.mock.calls.find(([,init])=>init?.method==="POST")!;expect(JSON.parse(call[1].body).occurred_at).toBeTruthy();
});
it("a failed preview cannot open a refund confirmation or show zero as an impact",async()=>{
 const fetcher=vi.fn(async(url)=>({ok:!String(url).endsWith("refund-preview"),json:async()=>String(url).endsWith("refund-preview")?{error:{message:"账务暂不可读"}}:detail}));vi.stubGlobal("fetch",fetcher);page();fireEvent.click(await screen.findByRole("button",{name:"登记线下退款"}));await screen.findByText("账务暂不可读");expect(screen.queryByRole("dialog")).toBeNull();
});

it.each(["refund_failed","refund_partial","refund_review","refunded","refunding"])("keeps %s provider refund facts visible without offering another refund",async(state)=>{
 const fetcher=vi.fn(async()=>({ok:true,json:async()=>({...detail,item:{...detail.item,order:{...order,refund_status:state,refund_amount_minor:2000},events:[{id:"ev1",adapter:"stripe",signature_valid:true,processed_at:"2026-10-10",status:state,processing_error:"local_application_failed"}]}})}));vi.stubGlobal("fetch",fetcher);page();await screen.findByText(/退款回报与本地账务需要核对/);expect(screen.queryByRole("button",{name:"登记线下退款"})).toBeNull();expect(screen.queryByRole("button",{name:"重试原退款"})).toBeNull();expect(await screen.findByText(/本地处理待恢复/)).toBeTruthy();
});

it("shows refunded credit as reclaimed and the readable original actors and brand",async()=>{
 vi.stubGlobal("fetch",vi.fn(async()=>({ok:true,json:async()=>({...detail,customer:{email:"alice@example.test",display_name:""},channel_names:{owner:"Aurora"},recorded_by:{email:"finance@example.test",display_name:"Finance"},refund_recorded_by:{email:"refund@example.test",display_name:"Refund Clerk"},item:{...detail.item,order:{...order,status:"refunded",recorded_by:"staff1",refund_recorded_by:"staff2",refunded_at:"2026-10-10T04:32:00Z"}}})})));page();
 expect(await screen.findByText("已收回原额度")).toBeTruthy();expect(screen.getByText("alice@example.test")).toBeTruthy();expect(screen.getByText("Aurora")).toBeTruthy();expect(screen.getByText("Finance")).toBeTruthy();expect(screen.getByText("Refund Clerk")).toBeTruthy();expect(screen.getByText(/12:32/)).toBeTruthy();expect(screen.queryByRole("button",{name:"登记线下退款"})).toBeNull();
});
