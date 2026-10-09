/** @vitest-environment jsdom */
import {cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import {afterEach,describe,expect,it,vi} from "vitest";
import WalletPanel from "@/app/console/wallet-panel";
import {WalletLedger} from "@/app/console/wallet/wallet-ledger";
import {EntitlementsPanel} from "@/app/console/entitlements-panel";
import {UserShellRightZone} from "@/components/layout/user-shell-menu";
import {withZh} from "./test-i18n";
vi.mock("next/navigation",()=>({usePathname:()=>"/app/wallet",useSearchParams:()=>new URLSearchParams(),useRouter:()=>({replace:vi.fn(),push:vi.fn(),refresh:vi.fn()})}));
vi.mock("@/components/rbac/viewer-context",()=>({useViewer:()=>({userId:"wallet-user",loading:false,roles:["end_user"]})}));
vi.mock("@/components/brand-context",()=>({useBrand:()=>({id:"wallet-brand"})}));
function response(body:unknown,status=200){return{ok:status<400,status,json:async()=>body};}
describe("wallet confirmed actions",()=>{
 afterEach(()=>{cleanup();sessionStorage.clear();vi.unstubAllGlobals();});
 it("keeps the redemption acknowledgement and refreshes header, ledger and allowances",async()=>{
  let redeemed=false;let ledgerReads=0;let allowanceReads=0;
  vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>{
   const url=String(input);
   if(url.includes("/v1/topups/redeem")){redeemed=true;return response({item:{amount_minor:10000000}});}
   if(url.includes("/v1/me/balance"))return response({balance:{available:redeemed?"10":"0",reserved:"0",purchased_minor:redeemed?10000000:0,gift_minor:0}});
   if(url.includes("/v1/me/wallet-records")){ledgerReads++;return response({items:redeemed?[{id:"new-credit",event_type:"topup",amount_minor:10000000,created_at:"2026-10-10"}]:[],total:redeemed?1:0,next_cursor:""});}
   if(url.includes("/v1/me/entitlements")){allowanceReads++;return response({items:[]});}
   if(url.includes("/v1/me"))return response({user:{display_name:"User",roles:["end_user"]}});
   if(url.includes("/v1/payments/checkout"))return response({item:{methods:[]}});
   return response({});
  }));
  render(withZh(<><UserShellRightZone/><WalletPanel/><WalletLedger/><EntitlementsPanel/></>));
  await waitFor(()=>expect(screen.getByTestId("balance-pill").textContent).toBe("$0.00"));
  await waitFor(()=>expect(allowanceReads).toBe(1));
  fireEvent.change(screen.getByLabelText("兑换码"),{target:{value:"THE2E"}});fireEvent.click(screen.getByRole("button",{name:"兑换",exact:true}));
  await waitFor(()=>expect(screen.getByTestId("balance-pill").textContent).toBe("$10.00"));
  expect(screen.getByText("已兑换到账 $10.00 USD。")).toBeTruthy();expect(screen.queryByText("余额已刷新")).toBeNull();
  await waitFor(()=>expect(screen.getByRole("cell",{name:"充值入账"})).toBeTruthy());
  expect(ledgerReads).toBeGreaterThanOrEqual(2);expect(allowanceReads).toBeGreaterThanOrEqual(2);
 });
 it("does not turn a confirmed redemption into failure when its balance refresh fails",async()=>{
  let redeemed=false;
  vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>{
   const url=String(input);
   if(url.includes("/v1/topups/redeem")){redeemed=true;return response({item:{amount_minor:10000000}});}
   if(url.includes("/v1/me/balance"))return redeemed?response({error:{message:"temporary"}},503):response({balance:{available:"0",reserved:"0",gift_minor:0,purchased_minor:0}});
   if(url.includes("/v1/payments/checkout"))return response({item:{methods:[]}});
   return response({});
  }));
  render(withZh(<WalletPanel/>));
  await waitFor(()=>expect(screen.getByText(/可用 0 USD/)).toBeTruthy());
  fireEvent.change(screen.getByLabelText("兑换码"),{target:{value:"THE2E"}});fireEvent.click(screen.getByRole("button",{name:"兑换",exact:true}));
  await waitFor(()=>expect(screen.getByRole("alert").textContent).toContain("余额暂未刷新"));
  expect(screen.getByRole("status").textContent).toContain("已兑换到账 $10.00 USD");
 });
});
