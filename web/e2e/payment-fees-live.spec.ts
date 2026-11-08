import {test,expect,type Page} from "@playwright/test";
import {readFileSync} from "node:fs";
const ready=process.env.TOKENHUB_FEE_ACCEPTANCE_FILE;
test.use({actionTimeout:10000});
test.skip(!ready,"Requires isolated fee acceptance app");
async function login(page:Page,email:string,password:string){await page.goto('/login');await page.getByLabel('邮箱',{exact:true}).fill(email);await page.getByLabel('密码',{exact:true}).fill(password);await page.getByRole('button',{name:'登录',exact:true}).click();await expect(page).toHaveURL(/\/(manage|workspace)(\/|$)/)}
test('one method configures collections and both withdrawals across four languages',async({browser},info)=>{
 test.setTimeout(150000);const f=JSON.parse(readFileSync(ready!,"utf8"));
 const staff=await browser.newContext({baseURL:process.env.TOKENHUB_WEB_ORIGIN,viewport:{width:1440,height:1000}}),customer=await browser.newContext({baseURL:process.env.TOKENHUB_WEB_ORIGIN,viewport:{width:1280,height:960}});
 const p=await staff.newPage(),c=await customer.newPage();
 try{
 await login(p,f.owner_email,f.owner_password);await p.goto(`/manage/${f.operator}/payment-methods`);
 await expect(p.getByRole('heading',{name:'支付方式与手续费',exact:true})).toBeVisible();
 await expect(p.getByText('Bank transfer',{exact:true}).first()).toBeVisible();
 await p.getByRole('button',{name:'设置方式与费用',exact:true}).click();
 const panel=p.getByRole('dialog');await expect(panel.getByLabel('支付方式名称',{exact:true})).toHaveValue('Bank transfer');
 await panel.getByLabel('手续费规则',{exact:true}).selectOption('fixed');await panel.getByLabel('固定手续费 USD',{exact:true}).fill('2');
 const confirm=panel.getByRole('checkbox');if(await confirm.count())await confirm.check();
 await panel.getByRole('button',{name:'设置方式与费用',exact:true}).click();await expect(panel.getByRole('status')).toBeVisible();await panel.getByRole('button',{name:'关闭办理面板',exact:true}).click();await expect(panel).toHaveCount(0);
 for(const lang of ['zh','en','ja','ru']){await p.getByTestId('language-selector').selectOption(lang);await expect(p.getByTestId('language-selector')).toHaveValue(lang);await p.screenshot({path:info.outputPath(`fee-settings-${lang}.png`),fullPage:true});expect(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)}
 await p.getByTestId('language-selector').selectOption('zh');await p.setViewportSize({width:390,height:844});await p.screenshot({path:info.outputPath('fee-settings-mobile.png'),fullPage:true});expect(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await p.setViewportSize({width:1440,height:1000});
 await login(c,f.customer_email,'customer-password-test');await c.goto('/workspace/topups');await c.getByRole('button',{name:'创建充值订单',exact:true}).click();
 const topup=c.getByRole('dialog');await topup.getByLabel('充值金额 USD',{exact:true}).fill('100');await topup.getByLabel('收款方式',{exact:true}).selectOption(f.merchant);
 await expect(topup.getByLabel('付款金额明细',{exact:true})).toBeVisible();await expect(topup.getByLabel('102 USD',{exact:true})).toBeVisible();await topup.getByRole('checkbox').check();
 const paymentResponse=c.waitForResponse(r=>r.url().endsWith('/api/v1/me/topups')&&r.request().method()==='POST');await topup.getByRole('button',{name:'创建充值订单',exact:true}).click();const payment=await(await paymentResponse).json();expect(payment.order.fee_quote.fee_money).toBe('2');expect(payment.order.payment_action.amount).toBe('102');
 await expect(c.getByRole('button',{name:'正在提交，请稍候…',exact:true})).toHaveCount(0);await c.getByLabel('付款金额明细',{exact:true}).scrollIntoViewIfNeeded();await c.screenshot({path:info.outputPath('topup-confirmed.png'),fullPage:true});
 // Record actual cash through the real HTTP handler; the customer-facing flow then reads its ledger result.
 const recorded=await p.evaluate(async({op,order})=>{const me=await(await fetch('/api/v1/auth/me')).json();const r=await fetch(`/api/v1/operators/${op}/payments`,{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':me.csrf_token,'Idempotency-Key':crypto.randomUUID()},body:JSON.stringify({order_id:order,currency:'USD',settlement_amount:'102',original_amount:'102',paid_at:new Date().toISOString()})});return{status:r.status,body:await r.json()}},{op:f.operator,order:payment.id});expect(recorded.status).toBe(200);
 const fulfilled=await p.evaluate(async({op,order})=>{const me=await(await fetch('/api/v1/auth/me')).json();const r=await fetch(`/api/v1/operators/${op}/orders/${order}/fulfill`,{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':me.csrf_token,'Idempotency-Key':crypto.randomUUID()},body:'{}'});return r.status},{op:f.operator,order:payment.id});expect(fulfilled).toBe(200);
 await p.goto(`/manage/${f.operator}/payments`);await expect(p.getByRole('button',{name:'核实手续费与结算'})).toHaveCount(0);await p.screenshot({path:info.outputPath('receipt-fee-recorded.png'),fullPage:true});
 const methods=await c.evaluate(async()=>{const result=[];for(const path of ['withdrawals','refunds']){const r=await fetch(`/api/v1/me/${path}/workspace`);result.push(await r.json())}return result});expect(methods[0].payout_methods[0].id).toBe(methods[1].payout_methods[0].id);expect(methods[0].payout_methods[0].flat_fee_money).toBe('2');
 await c.goto('/workspace/refunds');await expect(c.getByText('Bank transfer',{exact:false}).first()).toBeVisible();await c.getByLabel('提现金额 USD',{exact:false}).fill('100');await expect(c.getByLabel('98 USD',{exact:true}).first()).toBeVisible();await c.screenshot({path:info.outputPath('balance-withdrawal-shared-rule.png'),fullPage:true});
 await p.goto(`/manage/${f.operator}/balance-withdrawals`);await expect(p.getByRole('button',{name:'新增提现方式',exact:true})).toHaveCount(0);await expect(p.getByRole('button',{name:'提现方式',exact:true})).toHaveCount(0);
 }catch(error){await p.screenshot({path:info.outputPath('staff-failure.png'),fullPage:true});await c.screenshot({path:info.outputPath('customer-failure.png'),fullPage:true});await info.attach('staff-dom',{body:await p.locator('body').innerText(),contentType:'text/plain'});await info.attach('customer-dom',{body:await c.locator('body').innerText(),contentType:'text/plain'});throw error;}finally{await staff.close();await customer.close()}
});
