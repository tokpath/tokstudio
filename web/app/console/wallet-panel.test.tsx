/** @vitest-environment jsdom */
import { cleanup,fireEvent,render,screen,waitFor } from "@testing-library/react";
import { afterEach,beforeEach,expect,it,vi } from "vitest";
import { withZh } from "@/lib/test-i18n";
import WalletPanel from "./wallet-panel";
vi.mock("next/navigation",()=>({usePathname:()=>"/app/wallet",useSearchParams:()=>new URLSearchParams("next=%2Fmodels%2Fecho%3Ftab%3Dagent")}));
vi.mock("@/components/rbac/viewer-context",()=>({useViewer:()=>({signedIn:true,loading:false,userId:"wallet-user",roles:["end_user"]})}));
vi.mock("@/components/brand-context",()=>({useBrand:()=>({id:"wallet-brand"})}));
vi.mock("@/components/checkout-pay",()=>({CheckoutPay:({checkout}:{checkout:{order:{id:string}}})=><p>{checkout.order.id}</p>}));
const response=(body:unknown,status=200)=>Promise.resolve({ok:status<400,status,json:async()=>body});
const key="tokenhub_wallet_purchase:wallet-user:wallet-brand";
function stub(create:(init:RequestInit)=>ReturnType<typeof response>,lookup:()=>ReturnType<typeof response>){
  vi.stubGlobal("fetch",vi.fn((input:RequestInfo|URL,init?:RequestInit)=>{
    const url=String(input);
    if(url.includes("wallet-purchases"))return lookup();
    if(url.endsWith("/v1/payments/orders"))return create(init!);
    if(url.includes("/v1/payments/quote"))return response({item:{adapter:"stripe",pay_major:100,pay_currency:"USD",pay_minor:100000000,fee_minor:0,credit_minor:100000000}});
    if(url.endsWith("/v1/payments/checkout"))return response({item:{methods:[{adapter:"stripe",display_name:"Stripe",pay_currency:"USD"}],settings:{quick_amounts:[100,300]}}});
    return response({balance:{available_minor:0},items:[]});
  }));
}
beforeEach(()=>sessionStorage.clear());afterEach(()=>{cleanup();vi.unstubAllGlobals();});
it("keeps the original topup after HTTP 500 and a temporary 404 lookup, including reload",async()=>{
  const requests:RequestInit[]=[];
  stub(init=>{requests.push(init);return requests.length===1 ? response({error:{message:"provider timeout"}},500) : response({checkout:{order:{id:"original-wallet-order",adapter:"stripe",status:"pending",amount_minor:100000000,currency:"USD"},sandbox:true}},201);},()=>response({},404));
  const view=render(withZh(<WalletPanel/>));
  await waitFor(()=>expect(screen.getByTestId("wallet-pay").hasAttribute("disabled")).toBe(false));
  fireEvent.click(screen.getByTestId("wallet-pay"));
  await screen.findByText("provider timeout");
  const original=sessionStorage.getItem(key);expect(original).toBeTruthy();
  view.unmount();render(withZh(<WalletPanel/>));
  await screen.findByText("已有一笔充值操作，请先继续原操作或确认结果。");
  fireEvent.click(screen.getAllByRole("button",{name:"查询创建结果"})[0]);
  await screen.findByText("original-wallet-order");
  expect(requests).toHaveLength(2);expect(requests[1].headers).toEqual(requests[0].headers);expect(requests[1].body).toBe(requests[0].body);
});
it("retains a confirmed order ID when recovery lookup fails",async()=>{
  sessionStorage.setItem(key,JSON.stringify({id:"known-operation",adapter:"stripe",payMajor:100,orderId:"known-wallet-order"}));
  stub(()=>response({},500),()=>response({},500));render(withZh(<WalletPanel/>));
  await screen.findByText("已创建原订单：known-wallet-order");
  await waitFor(()=>expect(JSON.parse(sessionStorage.getItem(key)!).orderId).toBe("known-wallet-order"));
});
