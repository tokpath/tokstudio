import { expect,test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";
const order={id:"pay_acceptance",user_id:"usr_alice",user_email:"alice@example.test",channel_code:"official-a",adapter:"manual",purpose:"wallet",currency:"USD",amount_minor:2500000,credit_minor:2500000,created_at:"2026-09-18T00:00:00Z",status:"pending",fulfilled_at:""};
test("finance finds a user order, verifies receipt, and records a manual refund",async({page})=>{
 await mockViewer(page,{roles:["finance_admin"]});let current={...order};
 await page.route("**/admin/payments?**",r=>r.fulfill({json:{items:[current]}}));
 await page.route("**/admin/payments/pay_acceptance/confirm",async r=>{current={...current,status:"paid",fulfilled_at:"2026-09-18T00:05:00Z"};await r.fulfill({json:{item:current}})});
 await page.route("**/admin/payments/pay_acceptance/refund",async r=>{current={...current,status:"refunded"};await r.fulfill({json:{item:current}})});
 await page.goto("/admin/payments");await page.getByLabel("查找订单",{exact:true}).fill("alice@example.test");await page.getByRole("button",{name:"搜索订单",exact:true}).click();
 await expect(page.getByRole("cell",{name:"$2.50 USD 充值额度 $2.50 USD"})).toBeVisible();
 await page.getByRole("button",{name:"确认收款",exact:true}).click();await expect(page.getByRole("dialog")).toContainText("alice@example.test");await expect(page.getByRole("dialog")).toContainText("$2.50 USD");
 await page.getByRole("button",{name:"确认",exact:true}).click();await expect(page.getByRole("status").filter({hasText:"额度已到账"})).toBeVisible();
 await page.getByRole("button",{name:"登记退款",exact:true}).click();await expect(page.getByRole("dialog")).toContainText("不会转账");await page.getByRole("button",{name:"确认",exact:true}).click();
 await expect(page.getByRole("cell",{name:"已退款",exact:true})).toBeVisible();await expect(page.getByRole("button",{name:"登记退款",exact:true})).toHaveCount(0);
});
test("audit can inspect orders but has no financial action buttons",async({page})=>{
 await mockViewer(page,{roles:["audit_readonly"]});await page.route("**/admin/payments?**",r=>r.fulfill({json:{items:[order]}}));await page.goto("/admin/payments");await expect(page.getByText("alice@example.test",{exact:true})).toBeVisible();await expect(page.getByRole("button",{name:"确认收款",exact:true})).toHaveCount(0);
});
