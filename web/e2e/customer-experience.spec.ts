import {expect,test} from "@playwright/test";
import {mockViewer} from "./mock-viewer";
const customer=(n:number)=>({id:`customer-${n}`,email:`customer-${n}@example.test`,display_name:`Customer ${n}`,status:"active",channel_org_id:"brand-channel",channel_code:"Review channel",brand_id:"review-brand",brand_name:"Review brand",created_at:"2026-10-01T00:00:00Z",roles:["end_user"]});

test("customer search finds an old account and preserves filters and cursor through details and reload",async({page})=>{
 await mockViewer(page,{roles:["platform_admin"]});
 await page.route("**/api/admin/customer-scopes",route=>route.fulfill({json:{items:[{id:"brand-channel",code:"Review channel",brand_id:"review-brand",brand_name:"Review brand",can_create_professional:true}]}}));
 const searches:string[]=[];
 await page.route("**/api/admin/customers?**",route=>{const url=new URL(route.request().url());searches.push(url.search);const q=url.searchParams.get("q");const second=url.searchParams.get("cursor")==="second";route.fulfill({json:{items:q ? [customer(120)]:Array.from({length:25},(_,i)=>customer(i+(second?26:1))),total:q?1:165,limit:25,next_cursor:q?"":second?"third":"second"}});});
 await page.route("**/api/admin/customers/customer-*",route=>{const id=route.request().url().split("/").at(-1)!;route.fulfill({json:{item:customer(Number(id.split("-")[1])),permissions:{operations:true,finance:true,audit:true,manage:true,attribution:true,professional_create:true,record_payment:true,read_payments:true},usage:{confirmed_count:250,pending_count:3,confirmed_minor:25000000,refunded_minor:7000000},orders:{count:170,paid:160,pending:8,refunded:2},activity:{count:250,items:[{request_id:"request-review",public_model_id:"echo",status:"failed",started_at:"2026-10-01T00:00:00Z"}]},entitlements:[],errors:{credit:"余额暂不可用"}}});});
 await page.goto("/admin/users");
 await expect(page.getByText("匹配 165 位客户 · 本页 25 位")).toBeVisible();
 await page.getByLabel("搜索客户",{exact:true}).fill("customer-120@example.test");await page.getByRole("button",{name:"搜索",exact:true}).click();
 await expect(page.getByText("匹配 1 位客户 · 本页 1 位")).toBeVisible();
 await page.getByRole("link",{name:"Customer 120",exact:true}).click();
 await expect(page.getByRole("heading",{name:"Customer 120",exact:true})).toBeVisible();
 await expect(page.getByText("请求共 250 条 · 最近 1 条")).toBeVisible();await expect(page.getByText("余额暂不可用")).toBeVisible();
 const payment=page.getByRole("link",{name:"线下收款划拨",exact:true});const paymentURL=new URL(await payment.getAttribute("href") as string,"http://local.test");expect(paymentURL.searchParams.get("customer_id")).toBe("customer-120");expect(paymentURL.searchParams.get("return_to")).toContain("/admin/users/customer-120");
 await expect(page.getByRole("link",{name:"核对消费冲正",exact:true})).toHaveCount(0);
 const request=page.getByRole("link",{name:"查看原请求",exact:true});const requestURL=new URL(await request.getAttribute("href") as string,"http://local.test");expect(requestURL.pathname).toBe("/admin/usage/requests/request-review");expect(requestURL.searchParams.get("return_to")).toBe(new URL(page.url()).pathname+new URL(page.url()).search);
 await expect(page.getByRole("listitem").filter({hasText:"request-review"})).toContainText("调用失败");
 await page.getByRole("link",{name:"← 返回客户列表",exact:true}).click();await expect(page.getByLabel("搜索客户",{exact:true})).toHaveValue("customer-120@example.test");await page.reload();await expect(page.getByLabel("搜索客户",{exact:true})).toHaveValue("customer-120@example.test");
 await page.getByRole("button",{name:"清除搜索",exact:true}).click();await page.getByLabel("客户渠道",{exact:true}).selectOption("brand-channel");await page.getByRole("button",{name:"下一页",exact:true}).click();
 await page.getByRole("link",{name:"Customer 26",exact:true}).click();await page.getByRole("link",{name:"← 返回客户列表",exact:true}).click();await expect(page.getByRole("link",{name:"Customer 26",exact:true})).toBeVisible();await page.reload();
 const returned=new URL(page.url());expect(returned.searchParams.get("cursor")).toBe("second");expect(returned.searchParams.get("channel_id")).toBe("brand-channel");expect(returned.searchParams.get("viewer_scope")).toContain("usr_rbac:");await expect(page.getByRole("link",{name:"Customer 26",exact:true})).toBeVisible();expect(searches.some(query=>query.includes("customer-120"))).toBe(true);
 await page.screenshot({path:"test-results/t05-customer-list.png",fullPage:true});
});

for (const failure of ["network", "server", "rejected"] as const) test(`professional setup preserves the registered account after ${failure} failure`,async({page})=>{
 await mockViewer(page,{roles:["platform_admin"]});
 await page.route("**/api/admin/customers/customer-120",route=>route.fulfill({json:{item:customer(120),permissions:{operations:true,finance:false,audit:false,manage:false,attribution:false,professional_create:true,record_payment:false},usage:{confirmed_count:0,pending_count:0,confirmed_minor:0,refunded_minor:0},activity:{count:0,items:[]},errors:{}}}));
 await page.route("**/api/admin/customer-scopes",route=>route.fulfill({json:{items:[{id:"brand-channel",code:"Review channel",brand_id:"review-brand",brand_name:"Review brand",can_create_professional:true}]}}));
 const payloads:unknown[]=[];await page.route("**/api/admin/professional-customers",async route=>{payloads.push(route.request().postDataJSON());if(failure === "network")await route.abort("failed");else await route.fulfill({status:failure === "server" ? 503 : 409,json:{error:{code:failure === "server" ? "service_unavailable" : "customer_already_bound",message:failure === "server" ? "server response unknown" : "客户已有专业推广关系"}}});});
 await page.goto("/admin/users/customer-120");await page.getByRole("button",{name:"开通专业推广关系",exact:true}).click();await expect(page.getByLabel("所属渠道",{exact:true})).toHaveValue("brand-channel");await expect(page.getByLabel("所属渠道",{exact:true})).toBeDisabled();await expect(page.getByText("customer-120@example.test · Review brand / Review channel · 已注册，普通 API 客户",{exact:true})).toBeVisible();
 await page.getByRole("button",{name:"开通推广关系",exact:true}).click();await page.getByRole("button",{name:"确认",exact:true}).click();await page.getByRole("button",{name:"取消",exact:true}).click();await expect(page.getByLabel("所属渠道",{exact:true})).toHaveValue("brand-channel");expect(payloads).toEqual([{user_id:"customer-120",type:"agent",parent_id:""}]);await expect(page.getByRole("button",{name:"开通推广关系",exact:true})).toBeVisible();await expect(page.getByText(failure === "rejected" ? "客户已有专业推广关系" : "网络不可用，请重试。",{exact:true})).toBeVisible();
});
