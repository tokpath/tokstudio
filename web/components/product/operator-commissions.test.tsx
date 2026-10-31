// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { ResourcePage, Session } from "@/lib/contracts";
import { OperatorCommissionsScreen } from "./operator-commissions";

let permissions = ["*"];
vi.mock("./context", () => ({ useProduct: () => ({ session: {kind:"staff",staff:{id:"staff-one",email:"finance@example.test",display_name:"Finance"},workspaces:[{operator:{id:"operator-one",kind:"oem",brand_id:"brand-one",name:"OEM"},permissions}]} as Session }) }));
afterEach(() => { cleanup(); vi.unstubAllGlobals(); permissions = ["*"]; window.history.replaceState(null,"","/"); });
const page = (name:string, extra:Partial<ResourcePage> = {}):ResourcePage => ({title:name,description:"",items:[{id:name,status:"effective",values:{name}}],columns:[],operations:[],row_operations:[],...extra});
const response = (value:unknown) => new Response(JSON.stringify(value),{status:200,headers:{"Content-Type":"application/json"}});
function mount(record?:string) {
  const cache = new QueryClient({defaultOptions:{queries:{retry:false}}});
  render(<QueryClientProvider client={cache}><OperatorCommissionsScreen operatorID="operator-one" record={record}/></QueryClientProvider>);
}

it("puts rules, payout methods and applications in one entry and resets filters between tabs",async()=>{
  window.history.replaceState(null,"","/manage/operator-one/commissions?q=old&cursor=old-page");
  const calls:string[]=[];
  vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>{
    const url=String(input);calls.push(url);
    return response(page(url.includes("withdrawals")?(url.includes("view=methods")?"Bank transfer":"Payout application"):"Referral rule",{views:url.includes("withdrawals")?[{value:"requests",label:"提现申请"},{value:"methods",label:"提现方式"}]:undefined}));
  }));
  mount();await screen.findByText("Referral rule");
  expect(screen.getAllByRole("tab")).toHaveLength(3);
  expect(calls).toHaveLength(1);
  fireEvent.click(screen.getByRole("tab",{name:"提现方式"}));await screen.findByText("Bank transfer");
  expect(calls.at(-1)).toBe("/api/v1/operators/operator-one/withdrawals?view=methods");
  expect(screen.getAllByText("提现方式")).toHaveLength(1);
  expect(window.location.search).toContain("section=methods");
  expect(window.location.search).not.toContain("cursor=");
  fireEvent.click(screen.getByRole("tab",{name:"提现申请"}));await screen.findByText("Payout application");
  expect(calls.at(-1)).toBe("/api/v1/operators/operator-one/withdrawals?view=requests");
});

it("lets payout staff enter applications directly without reading marketing rules",async()=>{
  permissions=["withdrawals.read","withdrawals.manage"];
  window.history.replaceState(null,"","/manage/operator-one/commissions?section=requests&record=payout-one");
  const fetch=vi.fn(async()=>response(page("Original payout")));vi.stubGlobal("fetch",fetch);
  mount("payout-one");await screen.findByText("Original payout");
  expect(screen.queryByRole("tab",{name:"分佣规则"})).toBeNull();
  expect(screen.getByRole("tab",{name:"提现申请"}).getAttribute("aria-selected")).toBe("true");
  expect(fetch.mock.calls).toHaveLength(1);
  expect(screen.getByRole("link",{name:"返回全部记录"}).getAttribute("href")).toBe("/manage/operator-one/commissions?section=requests");
});

it("does not expose payout data to staff who manage only referral rules",async()=>{
  permissions=["commissions.read","marketing.manage"];
  window.history.replaceState(null,"","/manage/operator-one/commissions?section=methods");
  const calls:string[]=[];
  vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>{calls.push(String(input));return response(page("Referral rule"));}));
  mount();await screen.findByText("Referral rule");
  expect(screen.getAllByRole("tab")).toHaveLength(1);
  expect(calls.every(url=>!url.includes("withdrawals"))).toBe(true);
});

it("keeps an uncertain payout action mounted when switching tabs and retries the original action",async()=>{
  permissions=["withdrawals.read","withdrawals.manage"];
  window.history.replaceState(null,"","/manage/operator-one/commissions?section=requests");
  const attempts:{key:string|null;body:string}[]=[];
  vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL,init?:RequestInit)=>{
    if(init?.method==="POST"){
      expect(String(input)).toBe("/api/v1/operators/operator-one/withdrawals/payout-one/approve");
      attempts.push({key:new Headers(init.headers).get("Idempotency-Key"),body:String(init.body)});
      if(attempts.length===1)throw new TypeError("network interrupted");
      return response({id:"payout-one",status:"approved"});
    }
    return response(page("Payout application",{items:[{id:"payout-one",status:"requested",values:{name:"Payout application"},allowed_actions:["approve"]}],row_operations:[{name:"approve",label:"审核申请",method:"POST",path:"/api/v1/operators/operator-one/withdrawals/{id}/approve",fields:[]}]}));
  }));
  mount();fireEvent.click(await screen.findByRole("button",{name:"审核申请"}));
  fireEvent.click(screen.getByRole("button",{name:"审核申请",exact:true}));
  await screen.findByRole("button",{name:"重试本次操作"});
  fireEvent.click(screen.getByRole("tab",{name:"提现方式",hidden:true}));
  expect(window.location.search).toContain("section=requests");
  expect(screen.getByRole("dialog")).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"重试本次操作"}));
  await waitFor(()=>expect(attempts).toHaveLength(2));
  expect(attempts[1]).toEqual(attempts[0]);
  await screen.findByRole("button",{name:"本次操作已完成，关闭后查看最新记录"});
});
