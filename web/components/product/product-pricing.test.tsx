// @vitest-environment jsdom
import { useState } from "react";
import { render, screen, within, fireEvent, cleanup, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { BillingTemplateEditor } from "./billing-template-editor";
import { CustomerPlans } from "./customer-plans";
import { ConfigurationEditor } from "./operation-config-editor";
import { configurationSchema, configDefaults } from "./operation-config";
import type { JSONValue } from "@/lib/contracts";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it("does not ask a channel to authorize children or the platform to choose an upstream price", () => {
  const op={name:"listing",label:"上架与下架",method:"POST" as const,path:"/api/v1/operators/channel/pricing",fields:[]};
  const schema=configurationSchema(op,"channel")!;
  const cache=new QueryClient({defaultOptions:{queries:{retry:false}}});
  vi.stubGlobal("fetch",vi.fn(async()=>new Response(JSON.stringify({items:[]}),{status:200})));
  const view=render(<QueryClientProvider client={cache}><ConfigurationEditor schema={schema} value={JSON.stringify(configDefaults(schema))} disabled={false} change={()=>{}} operatorID="channel" allowDownstream={false} /></QueryClientProvider>);
  expect(screen.queryByLabelText("办理事项",{exact:false})).toBeNull();
  expect(screen.queryByText("被授权的直属下级",{exact:false})).toBeNull();
  const price=configurationSchema({...op,name:"price"},"platform")!;
  view.rerender(<QueryClientProvider client={cache}><ConfigurationEditor schema={price} value={JSON.stringify(configDefaults(price))} disabled={false} change={()=>{}} operatorID="platform" allowFollow={false} /></QueryClientProvider>);
  expect(screen.queryByRole("option",{name:"跟随上级供货价"})).toBeNull();
  view.rerender(<QueryClientProvider client={cache}><ConfigurationEditor schema={price} value={JSON.stringify(configDefaults(price))} disabled={false} change={()=>{}} operatorID="channel" allowDownstream={false} /></QueryClientProvider>);
  expect(screen.queryByRole("option",{name:"对下级供货价"})).toBeNull();
});

it("lets operators combine input media without keeping an overlapping aggregate charge", () => {
  function Form() {
    const [value, setValue] = useState<JSONValue>({input:[{modality:"all",template:"tokens"}],output:[{modality:"all",template:"tokens"}]});
    return <><BillingTemplateEditor value={value as Record<string,JSONValue>} disabled={false} change={setValue} variants={() => null} /><output data-testid="value">{JSON.stringify(value)}</output></>;
  }
  render(<Form />);
  const input = screen.getByText("输入计量",{exact:true}).closest("fieldset")!;
  fireEvent.click(within(input).getByRole("radio",{name:"按模态组合"}));
  fireEvent.click(screen.getByRole("checkbox",{name:"输入计量 · 图片"}));
  fireEvent.change(screen.getByRole("combobox",{name:"输入计量 · 图片 · 计量方式"}),{target:{value:"megapixels"}});
  expect(JSON.parse(screen.getByTestId("value").textContent!)).toMatchObject({input:[{modality:"text",template:"tokens"},{modality:"image",template:"megapixels"}]});
  expect(screen.getByRole("combobox",{name:"输入计量 · 图片 · 计量方式"}).textContent).not.toContain("按字符");
  fireEvent.click(within(input).getByRole("radio",{name:"统一 Token"}));
  expect(JSON.parse(screen.getByTestId("value").textContent!).input).toEqual([{modality:"all",template:"tokens"}]);
  expect(screen.queryByRole("checkbox",{name:"输入计量 · 图片"})).toBeNull();
});

it("shows no renewal form for lifetime credits and binds changed-term consent to the displayed quote", async () => {
  const quote={quote_id:"current-quote",plan_id:"package",name:"月套餐",price_money:"20",purchase_atg:"100",gift_atg:"20",validity:"month",terms_summary:"本次续费服务约定"};
  const base={name:"月套餐",status:"active",purchase_remaining_atg:"5",gift_remaining_atg:"1",effective_at:"2026-10-01T00:00:00Z",expires_at:"2030-11-01T00:00:00Z",auto_renew:false,auto_renew_available:true,allow_commission:false,requires_confirmation:true,renewal_quote:quote,version:1};
  const sent:unknown[]=[];
  vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL, init?:RequestInit) => {
    if(init?.method==="PATCH") {sent.push(JSON.parse(String(init.body)));return new Response(JSON.stringify({id:"renewal",status:"updated"}),{status:200});}
    return new Response(JSON.stringify(String(input).endsWith("/catalog") ? {items:[]} : {items:[{...base,id:"lifetime-order",name:"不限期套餐",expires_at:null,auto_renew_available:false,requires_confirmation:false,renewal_unavailable_reason:"不限期套餐无需续费，可随时另购套餐增加额度。"},{...base,id:"monthly-order"}]}),{status:200});
  }));
  const cache=new QueryClient({defaultOptions:{queries:{retry:false}}});
  render(<QueryClientProvider client={cache}><CustomerPlans /></QueryClientProvider>);
  await screen.findByText("不限期套餐无需续费，可随时另购套餐增加额度。");
  expect(screen.getAllByRole("button",{name:"保存续费设置"})).toHaveLength(1);
  expect(screen.queryByText("lifetime-order")).toBeNull();
  expect(screen.getByText("本次续费服务约定")).toBeTruthy();
  fireEvent.click(screen.getByRole("checkbox",{name:"到期自动续费"}));
  expect((screen.getByRole("button",{name:"保存续费设置"}) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(screen.getByRole("checkbox",{name:"确认以上新条款"}));
  fireEvent.click(screen.getByRole("button",{name:"保存续费设置"}));
  await waitFor(()=>expect(sent).toHaveLength(1));
  expect(sent[0]).toMatchObject({auto_renew:true,confirmed_changed_terms:true,quote_id:"current-quote",allow_commission:false});
});
