/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { withZh } from "@/lib/test-i18n";
import PlansPanel from "./plans-panel";

const state = vi.hoisted(() => ({userId:"buyer-a"}));
vi.mock("next/navigation",()=>({usePathname:()=>"/app/plans",useSearchParams:()=>new URLSearchParams("plan=monthly&next=%2Fmodels%2Fecho%3Ftab%3Dagent")}));
vi.mock("@/components/rbac/viewer-context",()=>({useViewer:()=>({signedIn:true,loading:false,userId:state.userId,roles:["end_user"]})}));
vi.mock("@/components/brand-context",()=>({useBrand:()=>({id:"brand-a"})}));
vi.mock("./entitlements-panel",()=>({EntitlementsPanel:()=>null}));
vi.mock("@/components/checkout-pay",()=>({CheckoutPay:({checkout,onPaid}:{checkout:{order:{id:string}},onPaid:()=>void})=><div><span>{checkout.order.id}</span><button onClick={onPaid}>confirm paid</button></div>}));
const plan={id:"monthly",name:"Monthly plan",price_minor:2_000_000,currency:"USD",billing_period:"monthly",auto_renew_allowed:true,items:[{unit_type:"usd_credit",included_amount:3_000_000,expires_in_seconds:86400*30}]};
const json=(body:unknown,status=200)=>Promise.resolve({ok:status<400,status,json:async()=>body});
function stub(post:(init?:RequestInit)=>ReturnType<typeof json>,recover:(url:string)=>ReturnType<typeof json>=()=>json({item:{id:"original-order",status:"pending"}})){
  const fetcher=vi.fn((input:RequestInfo|URL,init?:RequestInit)=>{
    const url=String(input);
    if(url.endsWith("/v1/me/plans"))return json({items:[plan]});
    if(url.endsWith("/v1/payments/checkout"))return json({item:{methods:[{adapter:"stripe",display_name:"Stripe",auto_renew_supported:true}]}});
    if(url.includes("subscription-purchases"))return recover(url);
    if(url.endsWith("/v1/me/subscriptions"))return post(init);
    return json({items:[]});
  });
  vi.stubGlobal("fetch",fetcher);return fetcher;
}
beforeEach(()=>{state.userId="buyer-a";sessionStorage.clear();});
afterEach(()=>{cleanup();vi.unstubAllGlobals();});

it("retains the original operation after a timeout and creates a fresh one after payment",async()=>{
  const requests:RequestInit[]=[];
  const fetcher=stub(init=>{requests.push(init!);return requests.length===1 ? Promise.reject(new Error("lost response")) : json({checkout:{order:{id:"original-order",status:"pending"},sandbox:true}},201);});
  render(withZh(<PlansPanel/>));
  const purchase=await screen.findByRole("button",{name:"订阅"});
  expect(screen.queryByRole("checkbox")).toBeNull();
  expect(screen.getByRole("link",{name:"返回原任务"}).getAttribute("href")).toBe("/models/echo?tab=agent");
  fireEvent.click(purchase);fireEvent.click(purchase);
  const retry=await screen.findByRole("button",{name:"继续原购买"});
  expect(requests).toHaveLength(1);
  expect(screen.getByRole("button",{name:"订阅"}).hasAttribute("disabled")).toBe(true);
  fireEvent.click(retry);
  await screen.findByText("original-order");
  expect(requests).toHaveLength(2);
  expect(requests[1].headers).toEqual(requests[0].headers);
  expect(requests[1].body).toBe(requests[0].body);
  expect(JSON.parse(String(requests[0].body)).auto_renew).toBe(false);
  fireEvent.click(screen.getByRole("button",{name:"confirm paid"}));
  await waitFor(()=>expect(screen.getByRole("button",{name:"订阅"}).hasAttribute("disabled")).toBe(false));
  fireEvent.click(screen.getByRole("button",{name:"订阅"}));
  await waitFor(()=>expect(requests).toHaveLength(3));
  expect(requests[2].headers).not.toEqual(requests[0].headers);
  expect(JSON.parse(String(requests[2].body)).auto_renew).toBe(false);
  expect(fetcher.mock.calls.some(([url])=>String(url).includes("subscription-purchases"))).toBe(true);
});

it("restores the original plan and payment parameters after reload",async()=>{
  sessionStorage.setItem("tokenhub_plan_purchase:buyer-a:brand-a",JSON.stringify({id:"saved-operation",planId:"original-plan",adapter:"alipay",autoRenew:false}));
  const requests:RequestInit[]=[];
  stub(init=>{requests.push(init!);return json({checkout:{order:{id:"resumed-order",status:"pending"},sandbox:true}},201);});
  render(withZh(<PlansPanel/>));
  fireEvent.click(await screen.findByRole("button",{name:"继续原购买"}));
  await screen.findByText("resumed-order");
  expect(requests[0].headers).toEqual({"Content-Type":"application/json","Idempotency-Key":"saved-operation"});
  expect(JSON.parse(String(requests[0].body))).toEqual({plan_id:"original-plan",adapter:"alipay",auto_renew:false});
});

it("ignores a late purchase response from the previous account",async()=>{
  let resolve!:(value:Awaited<ReturnType<typeof json>>)=>void;
  const late=new Promise<Awaited<ReturnType<typeof json>>>(done=>{resolve=done;});
  stub(()=>late);
  const view=render(withZh(<PlansPanel/>));
  fireEvent.click(await screen.findByRole("button",{name:"订阅"}));
  state.userId="buyer-b";view.rerender(withZh(<PlansPanel/>));
  await screen.findByRole("button",{name:"订阅"});
  resolve(await json({checkout:{order:{id:"other-account-order",status:"pending"},sandbox:true}},201));
  await waitFor(()=>expect(screen.queryByText("other-account-order")).toBeNull());
  expect(screen.queryByRole("button",{name:"继续原购买"})).toBeNull();
});
