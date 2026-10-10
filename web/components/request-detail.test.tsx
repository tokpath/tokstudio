/** @vitest-environment jsdom */
import {cleanup,render,screen} from "@testing-library/react";
import {QueryClient,QueryClientProvider} from "@tanstack/react-query";
import {afterEach,expect,it,vi} from "vitest";
import {RequestDetail} from "./request-detail";
import {withZh} from "@/lib/test-i18n";
vi.mock("@/components/rbac/viewer-context",()=>({useViewer:()=>({userId:"staff",signedIn:true,roles:["audit_readonly"]})}));
vi.mock("@/components/brand-context",()=>({useBrand:()=>({id:"brand"})}));
const back="/admin/usage?tab=requests&public_model_id=Echo&_admin_requests_q=original";
afterEach(()=>{cleanup();vi.unstubAllGlobals();sessionStorage.clear()});
it("keeps the entire return chain, uses the real customer route and states empty commission",async()=>{const self=`/admin/usage/requests/original?return_to=${encodeURIComponent(back)}`;window.history.replaceState(null,"",self);vi.stubGlobal("fetch",vi.fn(async()=>({ok:true,json:async()=>({request:{request_id:"original",public_model_id:"Echo",result:"succeeded",customer_amount_minor:10,started_at:"2026-10-10T00:00:00Z"},customer_id:"customer",customer:{id:"customer",email:"someone@example.test",display_name:"Customer"},attempts:[],charges:[],usage:[],commission_status:"ready",commissions:[]})})));render(<QueryClientProvider client={new QueryClient()}>{withZh(<RequestDetail surface="admin" id="original"/>)}</QueryClientProvider>);const customer=await screen.findByRole("link",{name:/查看客户/});const url=new URL(customer.getAttribute("href")!,"https://console.invalid");expect(url.pathname).toBe("/admin/users/customer");expect(url.searchParams.get("return_to")).toBe(self);expect(screen.getByRole("link",{name:/返回/}).getAttribute("href")).toBe(back);expect(screen.getByText("此请求未产生佣金。")).toBeTruthy()});
