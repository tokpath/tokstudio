import { expect,test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";
test("user administration requires a deliberate reason and keeps failure inside review",async({page})=>{
 await mockViewer(page,{roles:["platform_admin"]});
 await page.route("**/api/admin/users",r=>r.fulfill({json:{items:[{id:"alice",email:"alice@example.test",channel_org_id:"A",status:"active"},{id:"bob",email:"bob@example.test",status:"banned"}]}}));
 let attempts=0;await page.route("**/admin/users/alice/ban",r=>{expect(r.request().postDataJSON()).toEqual({reason:"验收测试"});attempts++;return r.fulfill({status:500,json:{error:{message:"佣金状态更新失败，已回滚。"}}});});
 await page.goto("/admin/users");await page.getByLabel("筛选用户").fill("alice");await expect(page.getByText("bob@example.test")).toHaveCount(0);
 await page.getByRole("button",{name:"封禁",exact:true}).click();await expect(page.getByLabel("操作原因",{exact:true})).toHaveValue("");await expect(page.getByRole("button",{name:"核对并继续"})).toBeDisabled();
 await page.getByLabel("操作原因",{exact:true}).fill("验收测试");await page.getByRole("button",{name:"核对并继续"}).click();const dialog=page.getByRole("dialog");await expect(dialog).toContainText("alice@example.test");await expect(dialog).toContainText("验收测试");await dialog.getByRole("button",{name:"确认",exact:true}).click();await expect(dialog.getByRole("alert")).toContainText("已回滚");expect(attempts).toBe(1);
});
test("commission check reports unchanged records and leaves conflicts recoverable",async({page})=>{
 await mockViewer(page,{roles:["finance_admin"]});let attempts=0;
 await page.route("**/admin/commissions/recalc",r=>{expect(r.request().postDataJSON()).toEqual({usage_event_id:"request-test"});return ++attempts===1?r.fulfill({status:409,json:{error:{message:"缺少原计算快照，未修改任何记录。"}}}):r.fulfill({json:{item:{changed:false,status:"frozen"}}});});
 await page.goto("/admin/commission");await page.getByLabel("消费请求编号或用量编号").fill("request-test");await page.getByRole("button",{name:"核对佣金",exact:true}).click();const dialog=page.getByRole("dialog");await dialog.getByRole("button",{name:"确认",exact:true}).click();await expect(dialog.getByRole("alert")).toContainText("未修改任何记录");await dialog.getByRole("button",{name:"确认",exact:true}).click();await expect(dialog).toHaveCount(0);await expect(page.getByRole("status").filter({hasText:"原佣金与计算快照一致"})).toBeVisible();
});
test("technical provider probe never presents unsupported real checks as healthy",async({page})=>{
 await mockViewer(page,{roles:["tech_admin"]});await page.route("**/api/admin/providers",r=>r.fulfill({json:{items:[{id:"real-provider",name:"Real Provider",slug:"real",adapter:"openai",health:"available",status:"active"}]}}));await page.route("**/admin/providers/real-provider/health-check",r=>r.fulfill({status:501,json:{error:{message:"此提供商尚不支持主动连通性探测；未修改路由健康标记。"}}}));await page.goto("/admin/providers");await page.getByRole("button",{name:"探测",exact:true}).click();await expect(page.getByRole("alert").filter({hasText:"未修改路由健康标记"})).toBeVisible();await expect(page.getByRole("button",{name:"探测",exact:true})).toBeEnabled();
});
test("route confirmation identifies target and new settings and retains API errors",async({page})=>{
 await mockViewer(page,{roles:["tech_admin"]});await page.route("**/api/admin/routes/qa_route",r=>r.request().method()==="GET"?r.fulfill({json:{item:{id:"qa_route",public_model_id:"test/model",strategy:"priority",status:"active",candidates:[{provider_id:"prd_a",provider_slug:"provider-a",upstream_model_id:"up-a",weight:1}]}}}):r.fulfill({status:400,json:{error:{message:"本地模拟保存失败，配置未变。"}}}));await page.goto("/admin/routes/qa_route");await page.getByRole("combobox",{name:"选路策略"}).selectOption({label:"选更健康的"});await page.getByRole("button",{name:"保存路由组",exact:true}).click();const dialog=page.getByRole("dialog").last();await expect(dialog).toContainText("test/model");await dialog.getByRole("button",{name:"确认",exact:true}).click();await expect(dialog.getByRole("alert")).toContainText("配置未变");
});
test("usage replay requires explicit actual quantities and shows dollar units",async({page})=>{
 await mockViewer(page,{roles:["finance_admin"]});
 await page.route("**/api/admin/usage?**",r=>r.fulfill({json:{items:[{id:"usage",request_id:"req_actual",state:"confirmed",customer_amount_minor:4000000}]}}));
 await page.route("**/api/admin/usage",r=>r.fulfill({json:{items:[{id:"usage",request_id:"req_actual",state:"confirmed",customer_amount_minor:4000000}]}}));
 await page.route("**/admin/usage/replay",r=>{expect(r.request().postDataJSON()).toEqual({request_id:"req_actual",usage:{prompt_tokens:3,completion_tokens:0}});return r.fulfill({status:409,json:{error:{message:"此请求无需再次对账。"}}});});
 await page.goto("/admin/usage");await expect(page.getByRole("cell",{name:"$4.00",exact:true})).toBeVisible();await expect(page.getByRole("cell",{name:"已结算",exact:true})).toBeVisible();await expect(page.getByLabel("真实输入 Token")).toHaveValue("");await page.getByLabel("消费请求编号",{exact:true}).fill("req_actual");await expect(page.getByRole("button",{name:"补录真实用量"})).toBeDisabled();await page.getByLabel("真实输入 Token").fill("3");await page.getByLabel("真实输出 Token").fill("0");await page.getByRole("button",{name:"补录真实用量"}).click();const dialog=page.getByRole("dialog");await expect(dialog).toContainText("req_actual");await expect(dialog).toContainText("输入 3 Token，输出 0 Token");await dialog.getByRole("button",{name:"确认",exact:true}).click();await expect(dialog.getByRole("alert")).toContainText("无需再次对账");
});
test("channel handoff names the channel and account and preserves rejected changes",async({page})=>{
 await mockViewer(page,{roles:["platform_admin"]});
 await page.route("**/api/admin/channels/test-b",r=>r.fulfill({json:{item:{id:"test-b",code:"Test B",type:"B",status:"active",brand_id:"brand"}}}));
 await page.route("**/api/admin/channels/test-b/admins",r=>r.request().method()==="GET"?r.fulfill({json:{items:null}}):r.fulfill({status:409,json:{error:{message:"用户不属于该渠道，未修改权限。"}}}));
 await page.goto("/admin/channels/test-b");await page.getByLabel("渠道管理员邮箱").fill("owner@example.test");await expect(page.getByRole("button",{name:"授权渠道管理"})).toBeDisabled();await page.getByLabel("权限变更原因").fill("渠道交接");await page.getByRole("button",{name:"授权渠道管理"}).click();const dialog=page.getByRole("dialog");await expect(dialog).toContainText("Test B");await expect(dialog).toContainText("owner@example.test");await dialog.getByRole("button",{name:"确认",exact:true}).click();await expect(dialog.getByRole("alert")).toContainText("未修改权限");
});
test("model publish readiness errors are visible inside confirmation",async({page})=>{
 await mockViewer(page,{roles:["ops_admin"]});await page.route("**/api/admin/models/test/self",r=>r.fulfill({json:{item:{id:"test/self",display_name:"Local draft",vendor:"test",status:"draft",capabilities:{kind:"text"}}}}));await page.route("**/admin/models/publish",r=>r.fulfill({status:409,json:{error:{message:"请先发布对应售价"}}}));await page.goto("/admin/models/test/self");await page.getByRole("button",{name:"发布模型",exact:true}).click();const dialog=page.getByRole("dialog");await dialog.getByRole("button",{name:"确认",exact:true}).click();await expect(dialog.getByRole("alert")).toContainText("对应售价");
});
