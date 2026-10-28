// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { ResourceScreen } from "./operator-resources";
import { CustomerRequests } from "./customer-records";

vi.mock("./context",()=>({useProduct:()=>({session:{kind:"staff",staff:{id:"staff",email:"staff@example.test",display_name:"负责人"},workspaces:[{operator:{id:"operator-one",name:"经营方",kind:"oem",brand_id:"brand"},permissions:["*"]}]}})}));
afterEach(()=>{cleanup();vi.unstubAllGlobals();window.history.replaceState(null,"","/");});
const response=(value:unknown)=>new Response(JSON.stringify(value),{status:200});
function mount(content:React.ReactNode){const cache=new QueryClient({defaultOptions:{queries:{retry:false,staleTime:Infinity,refetchOnWindowFocus:false}}});return render(<QueryClientProvider client={cache}>{content}</QueryClientProvider>);}
function page(name?:string,next?:string){return {title:"客户服务",description:"",search_supported:true,status_supported:true,items:name?[{id:name,values:{name},presentation:{title:name,labels:{}}}]:[],columns:[{key:"name",label:"名称",kind:"text"}],operations:[],row_operations:[],next_cursor:next};}
async function search(value:string){fireEvent.change(screen.getByLabelText("搜索记录"),{target:{value}});await waitFor(()=>expect((screen.getByRole("button",{name:"查询",exact:true}) as HTMLButtonElement).disabled).toBe(false));fireEvent.click(screen.getByRole("button",{name:"查询",exact:true}));}

it("re-reads unchanged empty and populated operator lists even while the cache is fresh",async()=>{
  let current:string|undefined;
  const fetch=vi.fn(async()=>response(page(current)));vi.stubGlobal("fetch",fetch);
  mount(<ResourceScreen operatorID="operator-one" resource="customers"/>);
  await screen.findByText("暂无记录");
  current="新增客户";await search("");await screen.findAllByText(current);expect(fetch).toHaveBeenCalledTimes(2);
  current="更新后的客户";await search("");await screen.findAllByText(current);expect(fetch).toHaveBeenCalledTimes(3);
});
it("refreshes a cached target filter without querying the previous filter",async()=>{
  let changed=false;
  const fetch=vi.fn(async(input:RequestInfo|URL)=>{const q=new URL(String(input),"http://localhost").searchParams.get("q");return response(page(q==="A"?(changed?"A最新":"A旧记录"):q==="B"?"B记录":undefined));});vi.stubGlobal("fetch",fetch);
  mount(<ResourceScreen operatorID="operator-one" resource="customers"/>);await screen.findByText("暂无记录");
  await search("A");await screen.findAllByText("A旧记录");await search("B");await screen.findAllByText("B记录");
  changed=true;await search("A");await screen.findAllByText("A最新");
  expect(fetch.mock.calls.map(call=>new URL(String(call[0]),"http://localhost").searchParams.get("q"))).toEqual([null,"A","B","A"]);
});
it("returns to and refreshes the first page on search without changing the filter",async()=>{
  let changed=false;
  const fetch=vi.fn(async(input:RequestInfo|URL)=>response(new URL(String(input),"http://localhost").searchParams.has("cursor")?page("第二页"):page(changed?"第一页最新":"第一页","next-page")));vi.stubGlobal("fetch",fetch);
  mount(<ResourceScreen operatorID="operator-one" resource="customers"/>);await screen.findAllByText("第一页");
  fireEvent.click(screen.getByRole("button",{name:"下一页",exact:true}));await screen.findAllByText("第二页");changed=true;
  await search("");await screen.findAllByText("第一页最新");
  expect(new URL(String(fetch.mock.calls.at(-1)?.[0]),"http://localhost").searchParams.has("cursor")).toBe(false);
  expect(screen.queryByRole("button",{name:"上一页",exact:true})).toBeNull();
});
it("refreshes unchanged customer record searches as well",async()=>{
  let hasRequest=false;
  const fetch=vi.fn(async()=>response({items:hasRequest?[{id:"request-one",model_id:"新模型调用",key_name:"应用",status:"completed",charged_atg:"1",held_atg:"0",created_at:"2026-10-28T00:00:00Z"}]:[]}));vi.stubGlobal("fetch",fetch);
  mount(<CustomerRequests/>);await screen.findByText("还没有模型调用");hasRequest=true;
  fireEvent.click(screen.getByRole("button",{name:"查询",exact:true}));await screen.findByRole("heading",{name:"新模型调用"});expect(fetch).toHaveBeenCalledTimes(2);
});
