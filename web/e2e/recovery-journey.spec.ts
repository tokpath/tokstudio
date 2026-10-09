import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";
const recovery={id:"claim_alice",user_id:"alice",settlement_id:"settlement_alice",created_at:"2026-10-10",amount_minor:300000,recovered_minor:0,status:"pending",recipient:{email:"alice@example.test",display_name:"Alice"},receipts:[]};
async function scope(page:import("@playwright/test").Page){await page.route("**/admin/commission-context",r=>r.fulfill({json:{owner_id:"platform",owner_code:"平台",channel_ids:[],channel_codes:{}}}));}
test("partial actual recovery retries one original operation with optional reference",async({page})=>{
 await mockViewer(page,{roles:["finance_admin"]});await scope(page);let received=false;const bodies:unknown[]=[];
 await page.route("**/admin/commission-recoveries?**",r=>r.fulfill({json:{items:[{...recovery,recovered_minor:received?100000:0}],total:1}}));await page.route("**/admin/commission-recoveries/claim_alice/receipts",async r=>{const body=r.request().postDataJSON();bodies.push(body);expect(body).toMatchObject({amount_minor:100000,confirmed:true,reference:"",note:"bank confirmed"});expect(body.idempotency_key).toBeTruthy();expect(body.occurred_at).toBeTruthy();received=true;if(bodies.length===1)await r.abort();else await r.fulfill({json:{item:{id:"receipt-original",recovery_id:"claim_alice",amount_minor:100000}}});});
 await page.goto("/admin/commission?tab=recovery");await page.getByRole("button",{name:"登记已收回款项",exact:true}).click();const drawer=page.getByRole("dialog");await drawer.getByLabel("本次实际收回金额（USD）").fill("0.4");await drawer.getByLabel("我确认这笔款项已实际收回").check();await expect(drawer.getByRole("button",{name:"核对收回款项"})).toBeDisabled();await drawer.getByLabel("本次实际收回金额（USD）").fill("0.1");await drawer.getByLabel("内部说明（可选）").fill("bank confirmed");await drawer.getByRole("button",{name:"核对收回款项"}).click();const confirm=page.getByRole("dialog").last();await expect(confirm).toContainText("剩余待收回: $0.20 USD");await confirm.getByRole("button",{name:"确认",exact:true}).click();await expect(confirm).toContainText("结果尚未确认");await confirm.getByRole("button",{name:"确认",exact:true}).click();await expect(page.getByText(/已确认登记：Alice/)).toBeVisible();expect(bodies).toHaveLength(2);expect(bodies[0]).toEqual(bodies[1]);
});
test("audit views completed recoveries without writable forms",async({page})=>{
 await mockViewer(page,{roles:["audit_readonly"]});await scope(page);await page.route("**/admin/commission-recoveries?**",r=>r.fulfill({json:{items:[{...recovery,status:"closed",recovered_minor:300000}],total:1}}));await page.goto("/admin/commission?tab=recovery&status=closed");await expect(page.getByRole("region",{name:"待收回"})).toContainText("已结清");await expect(page.getByRole("button",{name:"登记已收回款项",exact:true})).toHaveCount(0);await page.getByRole("button",{name:"查看详情"}).click();await expect(page.getByLabel("我确认这笔款项已实际收回")).toHaveCount(0);
});
test("receipt conflict stays visible and does not claim success",async({page})=>{
 await mockViewer(page,{roles:["finance_admin"]});await scope(page);await page.route("**/admin/commission-recoveries?**",r=>r.fulfill({json:{items:[recovery],total:1}}));await page.route("**/admin/commission-recoveries/claim_alice/receipts",r=>r.fulfill({status:409,json:{error:{message:"该参考号已登记，请刷新核对。"}}}));await page.goto("/admin/commission?tab=recovery");await page.getByRole("button",{name:"登记已收回款项",exact:true}).click();const drawer=page.getByRole("dialog");await drawer.getByLabel("交易参考号（可选）").fill("EXISTING");await drawer.getByLabel("我确认这笔款项已实际收回").check();await drawer.getByRole("button",{name:"核对收回款项"}).click();const confirm=page.getByRole("dialog").last();await confirm.getByRole("button",{name:"确认",exact:true}).click();await expect(confirm).toContainText("该参考号已登记");await expect(page.getByText(/已确认登记/)).toHaveCount(0);
});

test("user retains closed receipts without a pending recovery warning", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"] });
  await page.route("**/v1/me/balance**", r => r.fulfill({ json: { balance: { available: "5", reserved: "0", purchased_minor: 5000000, gift_minor: 0, commission_available_minor: 0, commission_recovery_minor: 0 } } }));
  await page.route("**/v1/me/commission-recoveries", r => r.fulfill({ json: { items: [{ ...recovery, status: "closed", recovered_minor: 300000, receipts: [{ id: "receipt", amount_minor: 300000, reference: "RETURN-CLOSED", created_at: "2026-09-18T00:00:00Z" }] }] } }));
  await page.goto("/app/wallet");
  await expect(page.getByLabel("余额组成").getByText("$5.00")).toBeVisible();
  const history = page.getByRole("region", { name: "佣金收回记录" });
  await expect(history).toContainText("已结清");
  await expect(history).toContainText("RETURN-CLOSED");
  await expect(history).toContainText("剩余 0 USD");
  await expect(page.getByRole("status").filter({ hasText: "待财务核对追回" })).toHaveCount(0);
});
