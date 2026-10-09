// @vitest-environment jsdom
import React from "react";
import {act,cleanup,render,screen,waitFor} from "@testing-library/react";
import {NextIntlClientProvider} from "next-intl";
import {QueryClient,QueryClientProvider} from "@tanstack/react-query";
import {afterEach,expect,it,vi} from "vitest";
import messages from "@/messages/zh.json";
import {CustomerList} from "./customer-list";
const state=vi.hoisted(()=>({user:"first",search:new URLSearchParams(),push:vi.fn(),replace:vi.fn()}));
const api=vi.hoisted(()=>vi.fn());
vi.mock("next/navigation",()=>({usePathname:()=>"/admin/users",useSearchParams:()=>state.search,useRouter:()=>({push:state.push,replace:state.replace})}));
vi.mock("@/components/rbac/viewer-context",()=>({useViewer:()=>({userId:state.user,loading:false,signedIn:true,roles:["platform_admin"]})}));
vi.mock("@/components/brand-context",()=>({useBrand:()=>({id:"brand"})}));
vi.mock("@/lib/client",()=>({apiClient:api}));
afterEach(()=>{cleanup();vi.clearAllMocks();state.user="first";state.search=new URLSearchParams();});
it("does not render the prior account's late customer response or restore its cursor",async()=>{
 let finishOld:(value:unknown)=>void=()=>{};const old=new Promise(resolve=>{finishOld=resolve});let directoryCalls=0;
 api.mockImplementation((_method:string,path:string)=>{if(path.includes("customer-scopes"))return Promise.resolve({items:[]});directoryCalls++;if(directoryCalls===1)return old;return Promise.resolve({items:[{id:"new",email:"new@example.test",status:"active",brand_name:"New brand",channel_code:"New channel",created_at:"2026-10-01"}],total:1,next_cursor:""});});
 const client=new QueryClient({defaultOptions:{queries:{retry:false}}});const ui=()=> <NextIntlClientProvider locale="zh" messages={messages}><QueryClientProvider client={client}><CustomerList surface="admin"/></QueryClientProvider></NextIntlClientProvider>;
 const mounted=render(ui());await waitFor(()=>expect(directoryCalls).toBe(1));
 state.user="second";state.search=new URLSearchParams("q=old&cursor=old-page&viewer_scope=first%3Abrand");mounted.rerender(ui());await waitFor(()=>expect(state.replace).toHaveBeenCalledWith("/admin/users"));
 await act(async()=>finishOld({items:[{id:"old",email:"old@example.test",status:"active",created_at:"2026-10-01"}],total:1,next_cursor:"old-page"}));expect(screen.queryByText("old@example.test")).toBeNull();
 state.search=new URLSearchParams();mounted.rerender(ui());await screen.findByText("new@example.test");expect(screen.queryByText("old@example.test")).toBeNull();const latest=api.mock.calls.filter(([,path])=>String(path).includes("/customers?" )).at(-1);expect(latest?.[1]).not.toContain("old-page");expect(latest?.[1]).not.toContain("q=old");
});
