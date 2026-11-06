// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { ResourceCollection } from "./operator-resources";
import type { ResourcePage } from "@/lib/contracts";

vi.mock("./context", () => ({ useProduct: () => ({ session: { kind: "staff", staff: { id: "staff-one" }, workspaces: [{ operator: { id: "operator-one", kind: "oem" }, permissions: ["operations.manage", "alerts.read"] }] } }) }));
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const page: ResourcePage = {title:"风险预警",items:["sales_space_atg","procurement_available_atg"].map(metric=>({id:metric,version:8,status:"warning",values:{metric,value:"50.000000000000000001",warning:"100",recipients:[],reminder_minutes:"60",enabled:true,default_owner_ids:["owner-one"],staff_choices:[{value:"owner-one",label:"负责人"},{value:"staff-two",label:"财务人员"}]}})),columns:[],operations:[],row_operations:[]};
function show(){return render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><ResourceCollection operatorID="operator-one" resource="alerts" page={page}/></QueryClientProvider>);}
it("shows two inline cards with active defaults and saves exact values without creating another rule",async()=>{
 const fetch=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>new Response(JSON.stringify({id:"sales_space_atg",status:"saved",version:9}),{status:200}));vi.stubGlobal("fetch",fetch);show();
 expect(screen.queryByRole("dialog")).toBeNull();expect(screen.queryByRole("button",{name:"新增预警规则"})).toBeNull();
 expect(screen.getAllByRole("button",{name:"保存预警设置"})).toHaveLength(2);
 const card=within(screen.getByRole("region",{name:"剩余销售空间ATG"}));
 expect((card.getByLabelText("启用预警") as HTMLInputElement).checked).toBe(true);
 expect((card.getByLabelText("跟随本经营方负责人") as HTMLInputElement).checked).toBe(true);
 fireEvent.change(card.getByLabelText("提醒阈值（ATG）"),{target:{value:"12.000000000000000001"}});
 fireEvent.change(card.getByLabelText("持续风险提醒间隔（分钟）"),{target:{value:"1.5"}});
 fireEvent.click(card.getByLabelText("财务人员"));fireEvent.click(card.getByRole("button",{name:"保存预警设置"}));
 await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(1));
 expect(JSON.parse(String(fetch.mock.calls[0][1]?.body))).toEqual({metric:"sales_space_atg",warning:"12.000000000000000001",recipients:["staff-two"],reminder_minutes:"1.5",enabled:true,expected_version:8});
 await card.findByText("预警设置已保存。");
});
it("keeps a network-ambiguous save locked and retries the original settings and action key",async()=>{
 let calls=0;const fetch=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>{calls++;if(calls===1)throw new TypeError("network lost");return new Response(JSON.stringify({id:"sales_space_atg",status:"saved",version:9}),{status:200});});vi.stubGlobal("fetch",fetch);show();
 const card=within(screen.getByRole("region",{name:"剩余销售空间ATG"}));const input=card.getByLabelText("提醒阈值（ATG）");
 fireEvent.change(input,{target:{value:"25"}});fireEvent.click(card.getByRole("button",{name:"保存预警设置"}));
 await card.findByRole("button",{name:"重试原设置"});expect(input.closest("fieldset")?.disabled).toBe(true);
 fireEvent.click(card.getByRole("button",{name:"重试原设置"}));await card.findByText("预警设置已保存。");
 expect(fetch.mock.calls[0][1]?.body).toBe(fetch.mock.calls[1][1]?.body);
 expect(fetch.mock.calls[0][1]?.headers).toEqual(fetch.mock.calls[1][1]?.headers);
});
it("rejects invalid thresholds and keeps unavailable measurements distinct from zero",()=>{
 vi.stubGlobal("fetch",vi.fn());const copy=structuredClone(page);delete copy.items[0].values.value;
 render(<QueryClientProvider client={new QueryClient()}><ResourceCollection operatorID="operator-one" resource="alerts" page={copy}/></QueryClientProvider>);
 const card=within(screen.getByRole("region",{name:"剩余销售空间ATG"}));expect(card.getByText("尚未核实")).toBeTruthy();
 fireEvent.change(card.getByLabelText("提醒阈值（ATG）"),{target:{value:"0"}});fireEvent.click(card.getByRole("button",{name:"保存预警设置"}));
 expect(card.getByRole("alert").textContent).toBe("提醒阈值必须大于 0 ATG。");expect(fetch).not.toHaveBeenCalled();
});
