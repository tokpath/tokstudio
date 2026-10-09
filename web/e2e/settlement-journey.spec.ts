import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";
const item = { id: "cst_alice", status: "settled", amount_minor: 1250000, created_at: "2026-10-10T00:00:00Z", period_start: "2026-10-01", period_end: "2026-11-01", recipient: { email: "alice@example.test", display_name: "Alice" }, channel_code: "CHANNEL-A", entries_snapshot_complete: true };
async function context(page: import("@playwright/test").Page) {
 await page.route("**/admin/commission-context",r=>r.fulfill({json:{owner_id:"platform",owner_code:"平台",channel_ids:["channel-a"],channel_codes:{"channel-a":"CHANNEL-A"}}}));
}
test("finance records actual payout and retains the exact operation after reload",async({page})=>{
 await mockViewer(page,{roles:["finance_admin"]});await context(page);
 await page.route("**/admin/settlements?**",r=>r.fulfill({json:{items:[item],total:1,next_cursor:""}}));
 await page.route("**/admin/settlements/cst_alice",r=>r.fulfill({json:{item,entries:[]}}));
 await page.route("**/admin/commission-operations/*",r=>r.fulfill({status:404,json:{error:{code:"not_found"}}}));
 const payloads:unknown[]=[];
 await page.route("**/admin/settlements/cst_alice/payout",async r=>{const body=r.request().postDataJSON();payloads.push(body);expect(body).toMatchObject({method:"manual",amount_minor:1250000,confirmed:true,reference:"",note:""});expect(body.operation_id).toBeTruthy();expect(body.occurred_at).toBeTruthy();expect(r.request().headers()["x-tokenhub-confirm"]).toBe("1");if(payloads.length===1)await r.abort();else await r.fulfill({json:{item:{...item,status:"paid",payout_id:"payout-original"}}});});
 await page.goto("/admin/commission?tab=payout");await page.getByRole("button",{name:"登记此单打款",exact:true}).click();const drawer=page.getByRole("dialog");await expect(drawer).toContainText("alice@example.test");await expect(drawer.getByRole("button",{name:"核对并登记"})).toBeDisabled();await drawer.getByLabel("我确认这笔打款已实际完成").check();await drawer.getByRole("button",{name:"核对并登记"}).click();await page.getByRole("dialog").last().getByRole("button",{name:"确认",exact:true}).click();await expect(page.getByRole("dialog").last()).toContainText("结果尚未确认");await page.reload();const original=page.getByLabel("原操作结果待确认");await original.getByRole("button",{name:"回查原操作"}).click();await expect(page.getByText(/暂未查到原记录/)).toBeVisible();await original.getByRole("button",{name:"重试原操作"}).click();await expect(page.getByText(/已确认登记：Alice/)).toBeVisible();expect(payloads).toHaveLength(2);expect(payloads[0]).toEqual(payloads[1]);
});
test("settlement preview preserves the minimum and submits the original candidate snapshot",async({page})=>{
 await mockViewer(page,{roles:["finance_admin"]});await context(page);await page.route("**/admin/commissions?**",r=>r.fulfill({json:{items:[],total:0}}));
 await page.route("**/admin/commissions/unfreeze",r=>{expect(r.request().postDataJSON().operation_id).toBeTruthy();return r.fulfill({json:{unfrozen:0}});});
 await page.route("**/admin/commissions/settlement-preview?**",r=>{expect(new URL(r.request().url()).searchParams.get("ignore_minimum")).toBeNull();return r.fulfill({json:{preview:{id:"preview-one",period_start:"2026-10-01",period_end:"2026-11-01",entry_count:3,recipient_count:1,settlement_count:1,amount_minor:1250000,min_settle_minor:1000000,ignore_minimum:false,policy_version:"version-one",groups:[],excluded:{frozen:{entry_count:2,amount_minor:100000}},expires_at:"2026-10-10T12:00:00Z"},recipients:{},channel_codes:{}}});});
 await page.route("**/admin/commissions/settle",r=>{const body=r.request().postDataJSON();expect(body.preview_id).toBe("preview-one");expect(body.operation_id).toBeTruthy();return r.fulfill({json:{items:[item],operation_id:body.operation_id}});});
 await page.goto("/admin/commission");await page.getByRole("button",{name:"解冻到期佣金",exact:true}).click();await page.getByRole("dialog").getByRole("button",{name:"确认",exact:true}).click();await expect(page.getByText(/已确认登记：解冻到期佣金/)).toBeVisible();await page.getByRole("button",{name:"结算预览",exact:true}).click();const drawer=page.getByRole("dialog");await expect(drawer.getByLabel("本次忽略最低结算额")).not.toBeChecked();await expect(drawer).toContainText("3 条佣金 · 1 位收款人");await expect(drawer).toContainText("冻结中 · 2");await drawer.getByRole("button",{name:"生成结算单",exact:true}).click();await page.getByRole("dialog").last().getByRole("button",{name:"确认",exact:true}).click();await expect(page.getByText(/已确认登记：生成结算单/)).toBeVisible();
});
test("audit reads original payout and cancellation facts without mutation controls",async({page})=>{
 await mockViewer(page,{roles:["audit_readonly"]});await context(page);const cancelled={...item,status:"cancelled",payout_reference:"WIRE-789",reversed_minor:300000};await page.route("**/admin/commissions?**",r=>r.fulfill({json:{items:[{...cancelled,settlement_id:item.id}],total:1}}));await page.route("**/admin/settlements/cst_alice",r=>r.fulfill({json:{item:cancelled,entries:[]}}));await page.goto("/admin/commission?tab=all");await page.getByRole("button",{name:"查看详情"}).click();await expect(page.getByRole("dialog")).toContainText("已撤销");await expect(page.getByRole("dialog")).toContainText("$0.30 USD");await expect(page.getByRole("button",{name:"核对并登记"})).toHaveCount(0);await expect(page.getByRole("button",{name:"结算预览",exact:true})).toHaveCount(0);
});

test("wallet links to personal earnings and shows recovery only in referral history", async ({ page }) => {
 await mockViewer(page,{roles:["end_user"]});
 await page.route("**/v1/me/balance**",r=>r.fulfill({json:{balance:{available:"5",reserved:"0",purchased_minor:5000000,gift_minor:0,commission_available_minor:0,commission_recovery_minor:300000}}}));
 await page.route("**/v1/me/wallet-records?kind=recoveries**",r=>r.fulfill({json:{items:[],total:0,next_cursor:""}}));
 await page.route("**/v1/me/referral**",r=>r.fulfill({json:{item:{codes:[],code_links:[],can_create:false,invited_count:0,can_commission:true,professional_customers:false,rules:{spend_minor:0,topup_minor:0,gift_minor:0},progress:{spend_minor:0,largest_topup_minor:0,gift_granted_minor:0,gift_remaining_minor:0},summary:{earned_minor:300000,frozen_minor:0,available_minor:0,held_minor:0,settled_minor:0,paid_minor:300000,reversed_minor:300000},rewards:[],settlements:[{id:"settlement_alice",status:"paid",amount_minor:300000,reversed_minor:300000,recovery_tracked:true,recovered_minor:0,recovery_pending_minor:300000}],pagination:{page:1,page_size:25,rewards_total:0,settlements_total:1}}}}));
 await page.goto("/app/wallet");
 await expect(page.getByLabel("余额组成").getByText("$5.00")).toBeVisible();
 await expect(page.getByText("佣金余额",{exact:true})).toHaveCount(0);
 await expect(page.getByRole("region",{name:"佣金收回记录"})).toHaveCount(0);
 await page.getByRole("link",{name:"邀请与收益",exact:true}).last().click();
 await page.getByRole("link",{name:"结算记录",exact:true}).click();
 await expect(page.getByRole("cell").filter({hasText:"待收回"})).toContainText("$0.30");
});
