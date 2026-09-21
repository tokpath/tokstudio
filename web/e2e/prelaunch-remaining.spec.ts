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
 await mockViewer(page,{roles:["tech_admin"]});await page.route("**/api/admin/routes",r=>r.fulfill({json:{items:[{id:"qa_route",public_model_id:"test/model",strategy:"priority",status:"active"}]}}));await page.route("**/api/admin/routes/qa_route",r=>r.fulfill({status:400,json:{error:{message:"本地模拟保存失败，配置未变。"}}}));await page.goto("/admin/routes");await page.getByRole("button",{name:"改策略",exact:true}).click();await page.getByRole("combobox",{name:"选路策略"}).selectOption({label:"选更健康的"});await page.getByRole("button",{name:"保存策略",exact:true}).click();const dialog=page.getByRole("dialog").last();await expect(dialog).toContainText("qa_route");await expect(dialog).toContainText("选更健康的");await dialog.getByRole("button",{name:"确认",exact:true}).click();await expect(dialog.getByRole("alert")).toContainText("配置未变");
});
