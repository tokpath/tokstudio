import { expect,test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

test("mobile navigation stays beneath its title and restores trigger focus after Escape",async({page})=>{
 await page.setViewportSize({width:390,height:844});
 await mockViewer(page,{roles:["platform_admin"]});
 await page.goto("/admin");
 const trigger=page.getByRole("button",{name:"打开导航",exact:true});
 await trigger.click();
 const drawer=page.getByRole("dialog");
 await expect(drawer).toBeVisible();
 const heading=await drawer.getByRole("heading").boundingBox();
 const nav=await drawer.getByRole("navigation").boundingBox();
 expect(heading).not.toBeNull();expect(nav).not.toBeNull();
 expect(nav!.y-(heading!.y+heading!.height)).toBeLessThan(48);
 await page.keyboard.press("Escape");
 await expect(drawer).not.toBeVisible();
 await expect(trigger).toBeFocused();
});

test("customer administration requires a deliberate reason and keeps failure inside review",async({page})=>{
 await mockViewer(page,{roles:["platform_admin"]});
 await page.route("**/api/admin/customers/alice",r=>r.fulfill({json:{item:{id:"alice",email:"alice@example.test",channel_org_id:"A",status:"active",created_at:"2026-10-10"},permissions:{manage:true},errors:{}}}));
 let attempts=0;await page.route("**/admin/users/alice/ban",r=>{expect(r.request().postDataJSON()).toEqual({reason:"验收测试"});attempts++;return r.fulfill({status:500,json:{error:{message:"佣金状态更新失败，已回滚。"}}});});
 await page.goto("/admin/users/alice");await page.getByRole("button",{name:"封禁客户",exact:true}).click();await expect(page.getByLabel("操作原因",{exact:true})).toHaveValue("");await expect(page.getByRole("button",{name:"核对并继续"})).toBeDisabled();
 await page.getByLabel("操作原因",{exact:true}).fill("验收测试");await page.getByRole("button",{name:"核对并继续"}).click();const dialog=page.getByRole("dialog");await expect(dialog).toContainText("alice@example.test");await expect(dialog).toContainText("验收测试");await dialog.getByRole("button",{name:"确认",exact:true}).click();await expect(dialog.getByRole("alert")).toContainText("已回滚");expect(attempts).toBe(1);
});
test("commission check reports unchanged records and leaves conflicts recoverable",async({page})=>{
 await mockViewer(page,{roles:["finance_admin"]});let attempts=0;
 await page.route("**/api/admin/commission-context",r=>r.fulfill({json:{owner_id:"official",owner_name:"Official",channel_ids:["official"],channel_codes:{official:"Official"}}}));
 await page.route("**/admin/commissions/recalc",r=>{expect(r.request().postDataJSON()).toEqual({usage_event_id:"request-test"});return ++attempts===1?r.fulfill({status:409,json:{error:{message:"缺少原计算快照，未修改任何记录。"}}}):r.fulfill({json:{item:{changed:false,status:"frozen"}}});});
 await page.goto("/admin/commission?tab=verify");await page.getByLabel("消费请求编号或用量编号").fill("request-test");await page.getByRole("button",{name:"核对佣金",exact:true}).click();const dialog=page.getByRole("dialog");await dialog.getByRole("button",{name:"确认",exact:true}).click();await expect(dialog.getByRole("alert")).toContainText("未修改任何记录");await dialog.getByRole("button",{name:"确认",exact:true}).click();await expect(dialog).toHaveCount(0);await expect(page.getByRole("status").filter({hasText:"原佣金与计算快照一致"})).toBeVisible();
});
test("technical provider probe never presents unsupported real checks as healthy",async({page})=>{
 await mockViewer(page,{roles:["tech_admin"]});await page.route("**/api/admin/providers",r=>r.fulfill({json:{items:[{id:"real-provider",name:"Real Provider",slug:"real",adapter:"openai",health:"unknown",status:"active"}]}}));await page.route("**/admin/providers/real-provider/health-check",r=>r.fulfill({status:501,json:{error:{message:"此提供商尚不支持主动连通性探测；未修改路由健康标记。"}}}));await page.goto("/admin/providers");await page.getByRole("button",{name:"探测",exact:true}).click();await expect(page.getByRole("alert").filter({hasText:"未修改路由健康标记"})).toBeVisible();await expect(page.getByRole("button",{name:"探测",exact:true})).toBeEnabled();
});
test("route confirmation identifies target and new settings and retains API errors",async({page})=>{
 await mockViewer(page,{roles:["tech_admin"]});
 await page.route("**/api/admin/models?**",r=>r.fulfill({json:{items:[{id:"test/model",display_name:"Model",vendor:"test",status:"published",kind:"text"}]}}));
 await page.route("**/api/admin/providers?**",r=>r.fulfill({json:{items:[{id:"prd_a",slug:"provider-a",name:"Provider A",adapter:"openai",status:"active"}]}}));
 await page.route("**/api/admin/providers/prd_a/upstream-models",r=>r.fulfill({json:{items:[{upstream_model_id:"up-a",display_name:"Up A",status:"active",unit_costs:{input:"0.000001",output:"0.000002"}}]}}));
 await page.route("**/api/admin/routes/qa_route",r=>r.request().method()==="GET"?r.fulfill({json:{item:{id:"qa_route",public_model_id:"test/model",strategy:"priority",status:"active",candidates:[{provider_id:"prd_a",provider_slug:"provider-a",upstream_model_id:"up-a",weight:1}]}}}):r.fulfill({status:400,json:{error:{message:"本地模拟保存失败，配置未变。"}}}));await page.goto("/admin/routes/qa_route");await page.getByRole("combobox",{name:"选路策略"}).selectOption({label:"选更健康的"});await page.getByRole("button",{name:"保存路由组",exact:true}).click();const dialog=page.getByRole("dialog").last();await expect(dialog).toContainText("test/model");await dialog.getByRole("button",{name:"确认",exact:true}).click();await expect(dialog.getByRole("alert")).toContainText("配置未变");
});
test("actual-usage review belongs to the original request and never assumes quantities",async({page})=>{
 await mockViewer(page,{roles:["finance_admin"]});
 await page.route("**/api/admin/requests/req_actual",r=>r.fulfill({json:{request:{request_id:"req_actual",public_model_id:"test/model",result:"failed",billing_state:"pending_reconciliation",customer_amount_minor:0,started_at:"2026-10-10"},authorization:{id:"auth1",status:"pending_reconciliation",amount_minor:4000000},attempts:[],charges:[],commissions:[],commission_status:"ready",usage:[{id:"usage",request_id:"req_actual",state:"pending_reconciliation",customer_amount_minor:0}]}}));
 await page.route("**/admin/requests/req_actual/reconcile",r=>{expect(r.request().postDataJSON()).toEqual({usage:{prompt_tokens:3,completion_tokens:0}});return r.fulfill({status:409,json:{error:{message:"此请求无需再次对账。"}}});});
 await page.goto("/admin/usage/requests/req_actual");await expect(page.getByText("$4.00 USD").first()).toBeVisible();await expect(page.getByLabel("输入 tokens")).toHaveValue("");
 await page.getByRole("button",{name:"录入本次真实用量"}).click();await expect(page.getByRole("dialog")).toHaveCount(0);
 await page.getByLabel("输入 tokens").fill("3");await page.getByLabel("输出 tokens").fill("0");await page.getByRole("button",{name:"录入本次真实用量"}).click();const dialog=page.getByRole("dialog");await expect(dialog).toContainText("req_actual");await expect(dialog).toContainText("输入 tokens 3");await dialog.getByRole("button",{name:"确认",exact:true}).click();await expect(dialog.getByRole("alert")).toContainText("无需再次对账");
});
test("channel handoff selects a verified customer and keeps rejected changes visible",async({page})=>{
 await mockViewer(page,{roles:["platform_admin"]});
 await page.route("**/api/admin/channels/test-b",r=>r.fulfill({json:{item:{id:"test-b",code:"Test Channel",type:"B",status:"active",brand_id:"brand"}}}));
 await page.route("**/api/admin/channels/test-b/admins",r=>r.request().method()==="GET"?r.fulfill({json:{items:[]}}):r.fulfill({status:409,json:{error:{message:"用户不属于该渠道，未修改权限。"}}}));
 await page.route("**/api/admin/channels/test-b/admins/candidates?**",r=>r.fulfill({json:{items:[{user_id:"owner1",email:"owner@example.test"}]}}));
 await page.goto("/admin/channels/test-b");await page.getByLabel("接手人",{exact:true}).selectOption("owner@example.test");await expect(page.getByRole("button",{name:"授权管理员"})).toBeEnabled();await page.getByRole("button",{name:"授权管理员"}).click();const dialog=page.getByRole("dialog");await expect(dialog).toContainText("Test Channel");await expect(dialog).toContainText("owner@example.test");await dialog.getByRole("button",{name:"确认",exact:true}).click();await expect(dialog.getByRole("alert")).toContainText("未修改权限");
});
test("model publish readiness errors are visible inside confirmation",async({page})=>{
 await mockViewer(page,{roles:["ops_admin"]});await page.route("**/api/admin/models/test/self",r=>r.fulfill({json:{item:{id:"test/self",display_name:"Local draft",vendor:"test",status:"draft",capabilities:{kind:"text"}}}}));await page.route("**/admin/models/publish",r=>r.fulfill({status:409,json:{error:{message:"请先发布对应售价"}}}));await page.goto("/admin/models/test/self");await page.getByRole("button",{name:"发布模型",exact:true}).click();const dialog=page.getByRole("dialog");await dialog.getByRole("button",{name:"确认",exact:true}).click();await expect(dialog.getByRole("alert")).toContainText("对应售价");
});
