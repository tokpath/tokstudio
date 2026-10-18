// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { Operation, ResourcePage } from "@/lib/contracts";
import { ResourceCollection } from "./operator-resources";
import { OperationForm } from "./operation-form";
import { businessFieldHelp } from "./business-field-help";
import { readableResourceValue } from "@/lib/resource-presentation";

vi.mock("./context", () => ({ useProduct: () => ({ session: { kind: "staff", staff: {id:"staff"}, workspaces:[{operator:{id:"workspace"},permissions:["*"]}] } }) }));
afterEach(() => { cleanup();vi.unstubAllGlobals(); });
const rowID="260f9ca6-dbb4-4137-bbcc-0904c9d59c33", supplierID="5d461bcb-a676-4648-a1cf-3c56ebfb8571";
function show(node:React.ReactNode){return render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}>{node}</QueryClientProvider>);}
function response(body:unknown){return new Response(JSON.stringify(body),{status:200,headers:{"content-type":"application/json"}});}
function resource():ResourcePage {return {title:"下属经营方",description:"",items:[{id:rowID,version:7,values:{supplier_id:supplierID,buyer_id:"buyer",warning_buffer:"30",action:"create",expected_version:7},presentation:{title:"已签供货约定",subtitle:"华东经营公司",labels:{supplier_id:"云服务公司 · cloud@example.test",buyer_id:"渠道甲 · channel@example.test"}}}],columns:[{key:"supplier_id",label:"供货方",kind:"text"},{key:"buyer_id",label:"采购方",kind:"text"},{key:"warning_buffer",label:"预警缓冲",kind:"atg"},{key:"action",label:"action",kind:"text"},{key:"expected_version",label:"当前版本",kind:"text"}],operations:[],row_operations:[]};}

it("shows real relation labels and preserves the full original ID only in details",async()=>{
 const clipboard={writeText:vi.fn().mockResolvedValue(undefined)};vi.stubGlobal("navigator",{...navigator,clipboard});
 show(<ResourceCollection operatorID="workspace" resource="operators" embedded page={resource()} />);
 expect(screen.getByText("云服务公司 · cloud@example.test")).toBeTruthy();
 expect(screen.getByText("渠道甲 · channel@example.test")).toBeTruthy();
 expect(screen.queryByText(rowID)).toBeNull();expect(screen.queryByText(supplierID)).toBeNull();
 expect(screen.queryByRole("columnheader",{name:"action"})).toBeNull();expect(screen.queryByRole("columnheader",{name:"当前版本"})).toBeNull();
 fireEvent.click(screen.getByRole("button",{name:"查看详情"}));
 const detail=screen.getByRole("dialog");expect(within(detail).getByText(rowID)).toBeTruthy();
 fireEvent.click(within(detail).getByRole("button",{name:"复制编号"}));await waitFor(()=>expect(clipboard.writeText).toHaveBeenCalledWith(rowID));
});

it("opens the same business explanation from a column title without requiring hover",()=>{
 show(<ResourceCollection operatorID="workspace" resource="coverage" embedded page={resource()} />);
 fireEvent.click(screen.getByRole("button",{name:"了解预警缓冲"}));
 expect(screen.getByRole("dialog").textContent).toContain("超卖额度不计入");
 expect(screen.getByRole("dialog").textContent).toContain("小于或等于此值");
 expect(businessFieldHelp("coverage.warning")).toBe(businessFieldHelp("warning_buffer"));
});

it("restores a chosen reference beyond the first result page and reviews its name while submitting the ID",async()=>{
 const fetch=vi.fn(async(input:RequestInfo|URL,init?:RequestInit)=>init?.method==="POST"?response({id:rowID,status:"saved"}):response(String(input).includes(encodeURIComponent(supplierID))?{items:[{value:supplierID,label:"真实供货方 · legal@example.test"}]}:{items:[],next_cursor:"later"}));vi.stubGlobal("fetch",fetch);
 const op:Operation={name:"save",label:"保存约定",method:"POST",path:"/api/v1/operators/workspace/operators",fields:[{name:"supplier_id",label:"供货方",kind:"text",required:true,lookup_url:"/api/v1/operators/workspace/references/suppliers"}]};
 show(<OperationForm operation={op} operatorID="workspace" initialValues={{supplier_id:supplierID}} />);
 await screen.findByRole("option",{name:"真实供货方 · legal@example.test"});
 await waitFor(()=>expect(screen.getAllByText("真实供货方 · legal@example.test").length).toBeGreaterThan(1));
 expect(screen.queryByText(supplierID)).toBeNull();
 fireEvent.click(screen.getByRole("button",{name:"保存约定",exact:true}));
 await waitFor(()=>expect(fetch.mock.calls.some(call=>call[1]?.method==="POST")).toBe(true));
 const post=fetch.mock.calls.find(call=>call[1]?.method==="POST")!;expect(JSON.parse(String(post[1]?.body))).toEqual({supplier_id:supplierID});
});

it("selects risk recipients by name and submits their real IDs as an array",async()=>{
 const fetch=vi.fn(async()=>response({id:rowID,status:"saved"}));vi.stubGlobal("fetch",fetch);
 const op:Operation={name:"save",label:"保存提醒",method:"POST",path:"/api/v1/operators/workspace/risk",fields:[{name:"recipients",label:"通知负责人",kind:"csv",required:true,options:[{value:supplierID,label:"林负责人 · lin@example.test"},{value:rowID,label:"陈财务 · chen@example.test"}]}]};
 show(<OperationForm operation={op} operatorID="workspace" />);
 fireEvent.click(screen.getByRole("checkbox",{name:"林负责人 · lin@example.test"}));
 expect(screen.queryByText(supplierID)).toBeNull();fireEvent.click(screen.getByRole("button",{name:"保存提醒",exact:true}));
 await waitFor(()=>expect(fetch).toHaveBeenCalled());expect(JSON.parse(String((fetch.mock.calls[0] as unknown as [string,RequestInit])[1].body))).toEqual({recipients:[supplierID]});
});

it("keeps public model identifiers readable and never disguises an internal UUID as a short label",()=>{
 const row={id:rowID,values:{customer_id:supplierID,public_model_id:"text-pro-v1",purpose:`customer:atg:${supplierID}`}};
 expect(readableResourceValue(row,"customer_id")).toBe("关联客户");
 expect(readableResourceValue(row,"public_model_id")).toBe("text-pro-v1");
 expect(readableResourceValue(row,"purpose")).toBe("客户 ATG 权益");
 expect(row.values.customer_id).toBe(supplierID);
});

it("selects a saved upstream for supplier cost and derives its provider without asking for UUIDs",async()=>{
 const fetch=vi.fn(async(input:RequestInfo|URL)=>response({items:String(input).includes("references/routes")?[{value:rowID,label:"上游模型甲 · 标准协议 · text-pro-v1",defaults:{provider_id:supplierID}}]:[]}));vi.stubGlobal("fetch",fetch);
 show(<OperationForm operatorID="workspace" operation={{name:"price",label:"发布模型价格",method:"POST",path:"/api/v1/operators/workspace/pricing",fields:[{name:"configuration",label:"配置 JSON",kind:"json",required:true}]}} />);
 expect(screen.queryByRole("textbox",{name:/定价经营方或供应商/})).toBeNull();
 fireEvent.change(screen.getByRole("combobox",{name:/^价目用途/}),{target:{value:"supplier"}});
 const route=screen.getByRole("combobox",{name:/^供货采购方或成本上游路由/});
 await waitFor(()=>expect((route as HTMLSelectElement).disabled).toBe(false));
 fireEvent.change(route,{target:{value:rowID}});
 const value=JSON.parse((screen.getByLabelText("完整技术配置 JSON") as HTMLTextAreaElement).value);
 expect(value.price.scope).toEqual({kind:"supplier",seller_id:supplierID,buyer_id:rowID});
 expect(screen.queryByText(rowID)).toBeNull();expect(screen.queryByText(supplierID)).toBeNull();
});

it("explains the warning buffer inside the editable coverage form",()=>{
 show(<OperationForm operatorID="workspace" operation={{name:"oversell",label:"设置授权超卖与缓冲",method:"POST",path:"/api/v1/operators/workspace/coverage?target_operator_id=channel",fields:[{name:"warning",label:"预警缓冲 ATG",kind:"decimal",required:true}]}} />);
 fireEvent.click(screen.getByRole("button",{name:"了解预警缓冲 ATG"}));
 expect(screen.getByRole("dialog").textContent).toContain("超卖额度不计入");
});

it("scopes the export subject selector to its chosen business and clears a stale subject",async()=>{
 const fetch=vi.fn(async(_input:RequestInfo|URL)=>response({items:[{value:rowID,label:"林客户 · lin@example.test"}]}));vi.stubGlobal("fetch",fetch);
 const operation:Operation={name:"export",label:"创建私有导出",method:"POST",path:"/api/v1/operators/workspace/exports",fields:[{name:"kind",label:"导出业务",kind:"select",default_value:"ledger",options:[{value:"ledger",label:"账本"},{value:"requests",label:"请求"}]},{name:"owner_id",label:"限定客户或合作方（留空导出当前范围）",kind:"text",lookup_url:"/api/v1/operators/workspace/references/export-subjects"}]};
 show(<OperationForm operatorID="workspace" operation={operation} initialValues={{owner_id:rowID}} />);
 const subject=screen.getByRole("combobox",{name:"限定客户或合作方（留空导出当前范围）"});await waitFor(()=>expect((subject as HTMLSelectElement).disabled).toBe(false));
 expect(String(fetch.mock.calls[0]?.[0])).toContain("?kind=ledger&q=");
 fireEvent.change(screen.getByRole("combobox",{name:"导出业务"}),{target:{value:"requests"}});
 await waitFor(()=>expect(fetch.mock.calls.some(call=>String(call[0]).includes("?kind=requests&q="))).toBe(true));
 expect((subject as HTMLSelectElement).value).toBe("");
});

it("explains initial-password limits and rejects a short password before sending the account request",async()=>{
 const fetch=vi.fn();vi.stubGlobal("fetch",fetch);
 show(<OperationForm operatorID="workspace" operation={{name:"invite",label:"开通经营人员",method:"POST",path:"/api/v1/operators/workspace/staff",fields:[{name:"initial_password",label:"新账号初始密码",kind:"password"}]}} />);
 const input=screen.getByLabelText("新账号初始密码");expect((input as HTMLInputElement).minLength).toBe(10);
 fireEvent.change(input,{target:{value:"password1"}});fireEvent.click(screen.getByRole("button",{name:"开通经营人员",exact:true}));
 expect(screen.getByRole("alert").textContent).toContain("至少 10 位，最长 72 字节");expect(fetch).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole("button",{name:"了解新账号初始密码"}));expect(screen.getByRole("dialog").textContent).toContain("至少 10 位，最长 72 字节");
});
