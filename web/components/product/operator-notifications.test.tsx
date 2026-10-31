// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { Entity, ResourcePage } from "@/lib/contracts";
import { OperatorNotificationGroups } from "./operator-notifications";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const notice = (id:string,read:string,name:string) => ({id,title:"模型与计价计划变更",body:"价格将于明天生效。",recipient_name:name,recipient_email:`${id}@example.test`,recipient_id:id,read_status:read,state:"pending",created_at:"2026-10-30T08:33:00Z",system_generated:true});
const page={items:[{id:"personal",status:"grouped",values:{record_kind:"personal",unread_count:1,total_count:2}},{id:"catalog:one",status:"grouped",values:{record_kind:"event",title:"模型与计价计划变更",body:"价格将于明天生效。",system_generated:true,unread_count:1,total_count:2}}] as Entity[],row_operations:[]} as Pick<ResourcePage,"items"|"row_operations">;
function response(notices:ReturnType<typeof notice>[],next?:string){return {title:"通知明细",description:"",items:notices.map(n=>({id:n.id,status:n.read_status,values:n})),columns:[],operations:[],row_operations:[],next_cursor:next};}
function mount(){const cache=new QueryClient({defaultOptions:{queries:{retry:false}}});render(<QueryClientProvider client={cache}><OperatorNotificationGroups page={page} operatorID="operator-one" onRetry={()=>{}}/></QueryClientProvider>);}

it("loads recipient details only when opened and paginates the group",async()=>{
 const fetch=vi.fn(async(input:RequestInfo|URL)=>{const url=new URL(String(input),"https://example.test");return new Response(JSON.stringify(response(url.searchParams.has("cursor")?[notice("person-unread","read","未读的人"),notice("person-read","read","已读的人")]:[notice("person-unread","unread","未读的人")],url.searchParams.has("cursor")?undefined:"next")),{status:200});});vi.stubGlobal("fetch",fetch);mount();
 expect(fetch).not.toHaveBeenCalled();expect(screen.getByText("未读 1 人 / 共 2 人")).toBeTruthy();
 fireEvent.click(screen.getAllByRole("button",{name:"查看详情"})[1]);const dialog=screen.getByRole("dialog");
 await within(dialog).findByRole("heading",{name:"未读的人",level:3});
 expect(String(fetch.mock.calls[0][0])).toContain("group=catalog%3Aone");
 fireEvent.click(within(dialog).getByRole("button",{name:"加载更多"}));await within(dialog).findByRole("heading",{name:"已读的人",level:3});
 expect(within(dialog).getAllByRole("heading",{level:3}).map(n=>n.textContent)).toEqual(["未读的人","已读的人"]);
 expect(String(fetch.mock.calls[1][0])).toContain("cursor=next");
 expect(within(dialog).queryByRole("button",{name:"标为已读"})).toBeNull();
});

it("lets staff read their own notices without notification management permission",async()=>{
 let read=false;const fetch=vi.fn(async(_input:RequestInfo|URL,init?:RequestInit)=>{if(init?.method==="POST"){read=true;return new Response(JSON.stringify({id:"mine-unread",status:"updated"}),{status:200});}return new Response(JSON.stringify(response([notice("mine-unread",read?"read":"unread","")])),{status:200});});vi.stubGlobal("fetch",fetch);mount();
 fireEvent.click(screen.getAllByRole("button",{name:"查看详情"})[0]);const dialog=screen.getByRole("dialog");fireEvent.click(await within(dialog).findByRole("button",{name:"标为已读"}));
 await waitFor(()=>expect(fetch.mock.calls.some(([input,init])=>String(input)==="/api/v1/operators/operator-one/notifications/mine-unread/read"&&init?.method==="POST")).toBe(true));
 await waitFor(()=>expect(within(dialog).queryByRole("button",{name:"标为已读"})).toBeNull());expect(screen.getByText("未读 0 条")).toBeTruthy();
});
