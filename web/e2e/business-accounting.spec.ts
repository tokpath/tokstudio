import { expect, test, type Page } from "@playwright/test";
import { mockViewer } from "./mock-viewer";
test.use({ timezoneId: "UTC" });
const owner="chn_oem_c";
async function defaults(page:Page,roles:string[]){
 await page.route("**/api/**",r=>r.fulfill({status:503,json:{error:{message:"未加载此业务数据"}}}));
 await mockViewer(page,{roles,channelType:"C"});
}
async function platformDelivery(page:Page){
 await defaults(page,["finance_admin"]);
 await page.route(`**/api/admin/oem-deliveries/${owner}`,r=>r.fulfill({json:{item:{delivery:{channel_org_id:owner,phase:"configuring",version:1,sales_mode:"offline",sell_plans:false,handoff_evidence:{}},channel:{id:owner,code:"aurora"},brand:{id:"brand-oem",name:"Aurora",primary_domain:"aurora.test",api_domain:"api.aurora.test",admin_domain:"admin.aurora.test"},checks:[],administrators:[],ready:false,checked_at:"2026-10-10T00:00:00Z"}}}));
 await page.route(`**/api/admin/channels/${owner}/models`,r=>r.fulfill({json:{items:[],channel_type:"C"}}));
 await page.route(`**/api/admin/channel-quotas/${owner}`,r=>r.fulfill({json:{quota:{available_minor:0,issued_minor:0,consumed_minor:0}}}));
 await page.route(`**/api/admin/channel-quotas/${owner}/issue-rule`,r=>r.fulfill({json:{rule:{issue_ratio_bps:10000}}}));
}
test("actual receipt and credit sale preserve their original identity after an unknown result and can reverse an incorrect registration",async({page})=>{
 await platformDelivery(page);
 let current:any=null,first=true,credited=0,reclaimed=0;const posts:any[]=[];
 await page.route("**/api/admin/oem-purchases**",async r=>{
  const request=r.request(),url=new URL(request.url());
  if(request.method()==="POST"&&url.pathname.endsWith("/reverse")){
   const body=request.postDataJSON();expect(request.headers()["x-tokenhub-confirm"]).toBe("1");expect(body.reason).toBe("OEM 选择有误");reclaimed++;
   current={...current,status:"reversed",reversal_operation_id:body.operation_id,reversal_reason:body.reason,reversed_at:"2026-10-10T00:10:00Z"};await r.fulfill({json:{item:current}});return;
  }
  if(request.method()==="POST"){
   const body=request.postDataJSON();posts.push(body);expect(request.headers()["x-tokenhub-confirm"]).toBe("1");
   if(!current){credited++;current={id:"purchase-original",...body,status:"completed",completed_at:"2026-10-10T00:06:00Z"};}
   if(first){first=false;await r.abort("failed");return;}await r.fulfill({json:{item:current}});return;
  }
  if(url.pathname.endsWith("/operations")){await r.fulfill({status:404,json:{error:{message:"尚未查到原交易，请保留原操作核对。"}}});return;}
  await r.fulfill({json:{items:current?[current]:[],next_cursor:""}});
 });
 await page.goto(`/admin/oem-deliveries/${owner}#quota`);
 const form=page.locator("section#procurement");await expect(form.getByRole("heading",{name:"OEM 服务额度销售"})).toBeVisible();
 await form.getByLabel("实际收款金额",{exact:true}).fill("1000");await form.getByLabel("划入服务额度（USD）",{exact:true}).fill("900");await form.getByLabel("实际收款时间",{exact:true}).fill("2026-10-10T00:05");await form.getByLabel("我确认款项已实际收到",{exact:true}).check();
 await form.getByRole("button",{name:"登记收款并划入额度",exact:true}).click();let dialog=page.getByRole("dialog");await expect(dialog).toContainText("$1000.00 USD");await expect(dialog).toContainText("$900.00 USD");await expect(dialog).toContainText("2026/10/10 08:05:00");expect(posts).toHaveLength(0);await dialog.getByRole("button",{name:"取消",exact:true}).click();expect(posts).toHaveLength(0);
 await form.getByRole("button",{name:"登记收款并划入额度",exact:true}).click();await page.getByRole("dialog").getByRole("button",{name:"确认",exact:true}).click();await expect(page.getByRole("dialog")).toContainText("结果待确认");await page.getByRole("dialog").getByRole("button",{name:"取消",exact:true}).click();await page.reload();
 await expect(form.getByRole("button",{name:"重试原交易",exact:true})).toBeVisible();await expect(form.getByLabel("实际收款金额",{exact:true})).toHaveCount(0);
 await form.getByRole("button",{name:"核对原交易",exact:true}).click();await expect(form.getByRole("status")).toContainText("尚未查到原交易");await expect(form.getByRole("button",{name:"重试原交易",exact:true})).toBeVisible();
 await form.getByRole("button",{name:"重试原交易",exact:true}).click();await page.getByRole("dialog").getByRole("button",{name:"确认",exact:true}).click();await expect(form.getByRole("status")).toContainText("purchase-original 已完成");expect(posts).toHaveLength(2);expect(posts[0]).toEqual(posts[1]);expect(posts[0]).toMatchObject({oem_channel_org_id:owner,cash_currency:"USD",cash_amount_minor:1000000000,sale_amount_minor:1000000000,quota_amount_minor:900000000,external_reference:"",note:"",occurred_at:"2026-10-10T00:05:00.000Z"});expect(credited).toBe(1);
 await form.getByRole("button",{name:"撤销误录",exact:true}).click();await form.getByLabel("撤销原因",{exact:true}).fill("OEM 选择有误");await form.getByRole("button",{name:"撤销误录",exact:true}).click();dialog=page.getByRole("dialog");await expect(dialog).toContainText("不执行对外退款");await expect(dialog).toContainText("purchase-original");await dialog.getByRole("button",{name:"确认",exact:true}).click();await expect(form).toContainText("误录已撤销");await expect(form).toContainText("登记更正，不代表已退现金");expect(reclaimed).toBe(1);await expect(form.getByRole("button",{name:"撤销误录",exact:true})).toHaveCount(0);
});

test("OEM gross profit uses consumed wholesale cost while cash purchases remain separate and read only",async({page})=>{
 await defaults(page,["channel_admin"]);
 await page.route("**/api/channel/commission-context",r=>r.fulfill({json:{owner_id:owner,owner_name:"Aurora",brand_id:"brand-oem"}}));
 await page.route("**/api/channel/allocations?**",r=>r.fulfill({json:{items:[],total:0,next_cursor:""}}));
 await page.route("**/api/channel/pnl",r=>r.fulfill({json:{pnl:{channel_org_id:owner,business_type:"oem",pool_status:"available",sell_minor:150000000,model_cost_minor:100000000,margin_minor:50000000,marketing_minor:0,pnl_minor:50000000,supplier_minor:-1000000000,purchase_minor:1000000000,recharge_minor:1000000000,unconsumed_minor:900000000,consumed_minor:150000000}}}));
 await page.route("**/api/channel/oem-purchases?**",r=>r.fulfill({json:{items:[{id:"purchase-cny",status:"completed",oem_channel_org_id:owner,cash_currency:"CNY",cash_amount_minor:70000,sale_amount_minor:100000000,quota_amount_minor:120000000,occurred_at:"2026-10-10T00:05:00Z",completed_at:"2026-10-10T00:06:00Z"}],next_cursor:""}}));
 await page.goto("/channel/ledger");const metric=(name:string)=>page.locator("dl > div").filter({has:page.getByText(name,{exact:true})});await expect(metric("营业收入")).toContainText("$150.00 USD");await expect(metric("实际 API 成本")).toContainText("$100.00 USD");await expect(metric("API 毛利")).toContainText("$50.00 USD");await expect(metric("API 经营利润（扣营销）")).toContainText("$50.00 USD");
 await page.getByRole("button",{name:"平台采购记录",exact:true}).click();await expect(page.locator("#procurement")).toContainText("¥700.00 CNY");await expect(page.locator("#procurement")).toContainText("$100.00 USD");await expect(page.locator("#procurement")).toContainText("$120.00 USD");await expect(page.getByRole("button",{name:"登记收款并划入额度",exact:true})).toHaveCount(0);await expect(page.getByRole("button",{name:"撤销误录",exact:true})).toHaveCount(0);
});
