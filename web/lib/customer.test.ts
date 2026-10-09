import {describe,expect,it} from "vitest";
import {customerReturnHref} from "./customer";
describe("customer return scope",()=>{
 it("preserves valid search and cursor for the original account",()=>{const value="/admin/users?q=older&cursor=second&channel_id=B&viewer_scope=user%3Abrand";expect(customerReturnHref(value,"/admin/users","user:brand")).toBe(value);});
 it("rejects another account or console and external redirects",()=>{for(const value of ["/admin/users?viewer_scope=other%3Abrand","/channel/users?q=older","//evil.test/admin/users","/admin\\evil"]){expect(customerReturnHref(value,"/admin/users","user:brand")).toBe("/admin/users");}});
});
