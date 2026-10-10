import {describe,expect,it,vi,afterEach} from "vitest";
import {fetchKeyPages} from "./key-resources";
afterEach(()=>vi.unstubAllGlobals());
describe("complete authorized Key/model resources",()=>{
 it("follows cursors so model 101 and beyond remain selectable",async()=>{
  const mock=vi.fn(async input=>({ok:true,json:async()=>String(input).includes("cursor=after100")?{items:[{id:"model101"}]}:{items:Array.from({length:100},(_,i)=>({id:`model${i}`})),next_cursor:"after100"}}));vi.stubGlobal("fetch",mock);
  const result=await fetchKeyPages("/v1/public/models");expect(result.items).toHaveLength(101);expect(mock.mock.calls[1][0]).toContain("cursor=after100");
 });
 it("does not expose a partial list as complete after a later page fails",async()=>{
  vi.stubGlobal("fetch",vi.fn(async input=>({ok:!String(input).includes("cursor="),json:async()=>String(input).includes("cursor=")?{error:{message:"page failed"}}:{items:[{id:"one"}],next_cursor:"one"}})));
  expect(await fetchKeyPages("/v1/me/api-keys")).toMatchObject({ok:false,message:"page failed"});
 });
});
