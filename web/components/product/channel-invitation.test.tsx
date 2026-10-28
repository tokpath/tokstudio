// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { ChannelInvitationCard } from "./channel-invitation";
const state=vi.hoisted(()=>({kind:"channel",permissions:["customers.read"]}));
vi.mock("./context",()=>({useProduct:()=>({session:{kind:"staff",staff:{id:"staff",email:"staff@example.test",display_name:"负责人"},workspaces:[{operator:{id:"channel-one",name:"星河渠道",kind:state.kind,brand_id:"brand"},permissions:state.permissions}]}})}));
afterEach(()=>{cleanup();vi.unstubAllGlobals();state.kind="channel";state.permissions=["customers.read"];});
function mount(){return render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><ChannelInvitationCard operatorID="channel-one"/></QueryClientProvider>);}
it("copies the existing channel code and a registration link on its own brand domain",async()=>{
  const link="https://oem.example.test/register?invite=CHAN01";
  vi.stubGlobal("fetch",vi.fn(async()=>new Response(JSON.stringify({invite_code:"CHAN01",invite_url:link,brand_name:"OEM品牌",enabled:true,registration_open:true}),{status:200})));
  const writeText=vi.fn(async(_value:string)=>{});vi.stubGlobal("navigator",{clipboard:{writeText}});mount();
  await screen.findByText("CHAN01");
  fireEvent.click(screen.getByRole("button",{name:"复制渠道邀请码",exact:true}));await waitFor(()=>expect(writeText).toHaveBeenCalledWith("CHAN01"));
  fireEvent.click(screen.getByRole("button",{name:"复制渠道邀请链接",exact:true}));await waitFor(()=>expect(writeText).toHaveBeenCalledWith(link));
  expect(screen.getByText(/不产生用户邀请上下级关系/)).toBeTruthy();
});
it("explains missing brand domains and closed registration without inventing a link",async()=>{
  vi.stubGlobal("fetch",vi.fn(async()=>new Response(JSON.stringify({invite_code:"CODE01",invite_url:"",brand_name:"OEM品牌",enabled:true,registration_open:false}),{status:200})));mount();
  await screen.findByText("CODE01");expect(screen.getByText(/品牌尚未配置可用域名/)).toBeTruthy();expect(screen.getByText(/当前渠道暂未开放注册/)).toBeTruthy();expect(screen.queryByRole("button",{name:"复制渠道邀请链接",exact:true})).toBeNull();
});
it("does not fetch or show a channel code for brand owners or staff without customer access",()=>{
  const fetch=vi.fn();vi.stubGlobal("fetch",fetch);state.kind="oem";const first=mount();expect(fetch).not.toHaveBeenCalled();first.unmount();
  state.kind="channel";state.permissions=["payments.manage"];mount();expect(fetch).not.toHaveBeenCalled();expect(screen.queryByText("渠道邀请")).toBeNull();
});
