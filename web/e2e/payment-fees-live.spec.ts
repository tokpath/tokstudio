import {test,expect,type Page} from "@playwright/test";
import {existsSync,readFileSync} from "node:fs";
const ready=process.env.TOKENHUB_FEE_ACCEPTANCE_FILE;
test.use({actionTimeout:10000});
test.skip(!ready,"Requires isolated fee acceptance app");
async function login(page:Page,email:string,password:string){await page.goto('/login');await page.getByLabel('邮箱',{exact:true}).fill(email);await page.getByLabel('密码',{exact:true}).fill(password);await page.getByRole('button',{name:'登录',exact:true}).click();await expect(page).toHaveURL(/\/(manage|workspace)(\/|$)/)}
test('one method configures collections and both withdrawals across four languages',async({browser},info)=>{
 test.setTimeout(240000);await expect.poll(()=>existsSync(ready!),{timeout:60000}).toBe(true);const f=JSON.parse(readFileSync(ready!,"utf8"));
 const staff=await browser.newContext({baseURL:process.env.TOKENHUB_WEB_ORIGIN,viewport:{width:1440,height:1000}}),customer=await browser.newContext({baseURL:process.env.TOKENHUB_WEB_ORIGIN,viewport:{width:1280,height:960}});
 const p=await staff.newPage(),c=await customer.newPage();
 try{
 await login(p,f.owner_email,f.owner_password);await p.goto(`/manage/${f.operator}/payment-methods`);
 await expect(p.getByRole('heading',{name:'支付方式与手续费',exact:true})).toBeVisible();
 await expect(p.locator('section.th-card')).toHaveCount(0);
 await p.screenshot({path:info.outputPath('empty-payment-methods.png'),fullPage:true});
 await p.getByRole('button',{name:'添加支付方式',exact:true}).click();
 const native=p.getByRole('dialog');await native.getByLabel('支付方式',{exact:true}).selectOption('offline');
 await expect(native.getByLabel('支付方式',{exact:true})).toHaveValue('offline');await expect(native.getByLabel('用于收款',{exact:true})).toBeChecked();
 await expect(native.getByLabel('收款办理方式',{exact:true})).toHaveCount(0);await expect(native.getByLabel('收款信息（可选）')).toHaveCount(0);await expect(native.getByLabel('付款说明（可选）')).toHaveCount(0);
 for(const lang of ['zh','en','ja','ru']){await native.getByTestId('language-selector').selectOption(lang);await p.screenshot({path:info.outputPath(`add-offline-${lang}.png`),fullPage:true})}
 await native.getByTestId('language-selector').selectOption('zh');await native.getByRole('button',{name:'保存支付方式',exact:true}).click();await expect(native.getByRole('status')).toBeVisible();await native.getByRole('button',{name:'关闭办理面板',exact:true}).click();
 await expect(p.getByRole('heading',{name:'线下支付',exact:true})).toBeVisible();
 await p.getByRole('button',{name:'添加支付方式',exact:true}).click();
 const pendingPanel=p.getByRole('dialog');await pendingPanel.getByLabel('支付方式',{exact:true}).selectOption('kvoucher');await expect(pendingPanel.getByLabel(/线下退款方式/)).toHaveCount(0);await pendingPanel.getByRole('button',{name:'保存支付方式',exact:true}).click();await expect(pendingPanel.getByRole('status')).toBeVisible();await pendingPanel.getByRole('button',{name:'关闭办理面板',exact:true}).click();await expect(p.getByText('待配置',{exact:true}).last()).toBeVisible();
 await p.getByRole('button',{name:'添加支付方式',exact:true}).click();
 const panel=p.getByRole('dialog');
 await panel.getByLabel('支付方式',{exact:true}).selectOption('bank_transfer');
 await expect(panel.getByLabel('用于余额和佣金提现')).not.toBeChecked();
 await panel.getByLabel('用于收款').check();await panel.getByLabel('用于余额和佣金提现').check();
 await expect(panel.getByLabel('支付方式名称',{exact:true})).toHaveCount(0);
 await expect(panel.getByLabel('提现收款材料',{exact:true})).toHaveCount(0);
 for(const field of ['收款信息（可选）','付款说明（可选）','提现到账币种','提现兑换比例','可办理的提现网络','提现办理说明（可选）'])await expect(panel.getByLabel(field,{exact:true})).toHaveCount(0);
 await panel.getByLabel('手续费规则',{exact:true}).selectOption('fixed');await panel.getByLabel('固定手续费 USD',{exact:true}).fill('2');
 for(const lang of ['zh','en','ja','ru']){await panel.getByTestId('language-selector').selectOption(lang);await p.screenshot({path:info.outputPath(`open-method-${lang}.png`),fullPage:true})}
 await panel.getByTestId('language-selector').selectOption('zh');
 await panel.getByRole('button',{name:'保存支付方式',exact:true}).click();await expect(panel.getByRole('status')).toBeVisible();await panel.getByRole('button',{name:'关闭办理面板',exact:true}).click();await expect(panel).toHaveCount(0);
 await expect(p.getByText('银行转账',{exact:true}).first()).toBeVisible();
 await expect(p.getByRole('button',{name:'收款连接',exact:true})).toHaveCount(0);
 for(const lang of ['zh','en','ja','ru']){await p.getByTestId('language-selector').selectOption(lang);await expect(p.getByTestId('language-selector')).toHaveValue(lang);if(lang==='en')await expect(p.getByRole('heading',{name:'Bank transfer',exact:true})).toBeVisible();await p.screenshot({path:info.outputPath(`fee-settings-${lang}.png`),fullPage:true});expect(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)}
 await p.getByTestId('language-selector').selectOption('zh');await p.setViewportSize({width:390,height:844});await p.screenshot({path:info.outputPath('fee-settings-mobile.png'),fullPage:true});expect(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await p.setViewportSize({width:1440,height:1000});
 await login(c,f.customer_email,'customer-password-test');await c.goto('/workspace/topups');await c.getByRole('button',{name:'创建充值订单',exact:true}).click();
 const topup=c.getByRole('dialog');await expect(topup.getByRole('option',{name:'KVoucher',exact:true})).toHaveCount(0);await topup.getByLabel('充值金额 USD',{exact:true}).fill('100');await topup.getByLabel('收款方式',{exact:true}).selectOption({label:'银行转账'});
 await expect(topup.getByLabel('付款金额明细',{exact:true})).toBeVisible();await expect(topup.getByLabel('102 USD',{exact:true})).toBeVisible();await topup.getByRole('checkbox').check();
 const paymentResponse=c.waitForResponse(r=>r.url().endsWith('/api/v1/me/topups')&&r.request().method()==='POST');await topup.getByRole('button',{name:'创建充值订单',exact:true}).click();const payment=await(await paymentResponse).json();expect(payment.order.fee_quote.fee_money).toBe('2');expect(payment.order.payment_action.amount).toBe('102');
 await expect(c.getByRole('button',{name:'正在提交，请稍候…',exact:true})).toHaveCount(0);await c.getByLabel('付款金额明细',{exact:true}).scrollIntoViewIfNeeded();await c.screenshot({path:info.outputPath('topup-confirmed.png'),fullPage:true});
 // Record actual cash through the real HTTP handler; the customer-facing flow then reads its ledger result.
 const recorded=await p.evaluate(async({op,order})=>{const me=await(await fetch('/api/v1/auth/me')).json();const r=await fetch(`/api/v1/operators/${op}/payments`,{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':me.csrf_token,'Idempotency-Key':crypto.randomUUID()},body:JSON.stringify({order_id:order,currency:'USD',settlement_amount:'102',original_amount:'102',paid_at:new Date().toISOString()})});return{status:r.status,body:await r.json()}},{op:f.operator,order:payment.id});expect(recorded.status).toBe(200);
 const fulfilled=await p.evaluate(async({op,order})=>{const me=await(await fetch('/api/v1/auth/me')).json();const r=await fetch(`/api/v1/operators/${op}/orders/${order}/fulfill`,{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':me.csrf_token,'Idempotency-Key':crypto.randomUUID()},body:'{}'});return r.status},{op:f.operator,order:payment.id});expect(fulfilled).toBe(200);
 await p.goto(`/manage/${f.operator}/payments`);await expect(p.getByRole('button',{name:'核实手续费与结算'})).toHaveCount(0);await p.screenshot({path:info.outputPath('receipt-fee-recorded.png'),fullPage:true});
 const methods=await c.evaluate(async()=>{const result=[];for(const path of ['withdrawals','refunds']){const r=await fetch(`/api/v1/me/${path}/workspace`);result.push(await r.json())}return result});expect(methods[0].payout_methods[0].id).toBe(methods[1].payout_methods[0].id);expect(methods[0].payout_methods[0].flat_fee_money).toBe('2');
 await c.goto('/workspace/refunds');await expect(c.getByText('银行转账',{exact:false}).first()).toBeVisible();await c.getByLabel('提现金额 USD',{exact:false}).fill('100');await expect(c.getByText('98 USD',{exact:true}).first()).toBeVisible();await c.screenshot({path:info.outputPath('balance-withdrawal-shared-rule.png'),fullPage:true});

 // Add a manual PayPal payout without any collection credentials, using only the original fixed fee rule.
 await p.goto(`/manage/${f.operator}/payment-methods`);await p.getByRole('button',{name:'添加支付方式',exact:true}).click();
 const paypal=p.getByRole('dialog');await paypal.getByLabel('支付方式',{exact:true}).selectOption('paypal');await paypal.getByLabel('用于余额和佣金提现').check();
 await expect(paypal.getByText('客户填写的收款资料',{exact:true})).toHaveCount(0);await expect(paypal.getByLabel('收款信息（可选）')).toHaveCount(0);
 await expect(paypal.getByLabel('提现到账币种')).toHaveCount(0);await paypal.getByLabel('手续费规则',{exact:true}).selectOption('fixed');await paypal.getByLabel('固定手续费 USD',{exact:true}).fill('2');
 await paypal.getByRole('button',{name:'保存支付方式',exact:true}).click();await expect(paypal.getByRole('status')).toBeVisible();await paypal.getByRole('button',{name:'关闭办理面板',exact:true}).click();
 await c.reload();await c.getByLabel('提现金额 USD',{exact:false}).fill('40');await c.locator('label').filter({has:c.getByText('PayPal',{exact:true})}).first().getByRole('radio').check();
 await c.getByLabel('PayPal 邮箱',{exact:true}).fill('recipient@example.test');await expect(c.getByText('38 USD',{exact:true}).first()).toBeVisible();
 for(const lang of ['zh','en','ja','ru']){await c.getByTestId('language-selector').selectOption(lang);await c.screenshot({path:info.outputPath(`customer-payout-${lang}.png`),fullPage:true});expect(await c.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)}
 await c.getByTestId('language-selector').selectOption('zh');
 const withdrawalResponse=c.waitForResponse(r=>r.url().endsWith('/api/v1/me/balance-withdrawals')&&r.request().method()==='POST');
 await c.getByRole('button',{name:'提交提现申请',exact:true}).click();const withdrawal=await(await withdrawalResponse).json();expect(withdrawal.id).toBeTruthy();
 await p.goto(`/manage/${f.operator}/balance-withdrawals`);await expect(p.getByText('38 USD',{exact:true}).first()).toBeVisible();
 await p.getByRole('button',{name:'通过',exact:true}).first().click();const approval=p.getByRole('dialog');await approval.getByRole('checkbox').check();await approval.getByRole('button',{name:'通过',exact:true}).click();await expect(approval.getByRole('status')).toBeVisible();await approval.getByRole('button',{name:'关闭办理面板',exact:true}).click();
 await p.getByRole('button',{name:'登记付款',exact:true}).first().click();const paid=p.getByRole('dialog');await expect(paid.getByLabel('实际付款币种',{exact:true})).toHaveValue('USD');await expect(paid.getByLabel('实际原币付款金额',{exact:true})).toHaveCount(0);await p.screenshot({path:info.outputPath('original-payment-registration.png'),fullPage:true});
 await paid.getByLabel('实际付款时刻',{exact:true}).fill(new Date(Date.now()-60000).toISOString().slice(0,16));await paid.getByRole('checkbox').check();await paid.getByRole('button',{name:'登记付款',exact:true}).click();await expect(paid.getByRole('status')).toBeVisible();await paid.getByRole('button',{name:'关闭办理面板',exact:true}).click();
 await c.reload();await expect(c.getByText('已核实付款',{exact:true})).toBeVisible();await c.goto('/workspace/withdrawals');await expect(c.getByText('PayPal',{exact:true}).first()).toBeVisible();
 for(const role of f.roles){
   const context=await browser.newContext({baseURL:process.env.TOKENHUB_WEB_ORIGIN,viewport:{width:1365,height:1000}});const r=await context.newPage();
   await login(r,role.email,role.password);await r.goto(`/manage/${role.operator}/payment-methods`);
   await expect(r.getByRole('heading',{name:'线下支付',exact:true})).toHaveCount(0);await expect(r.locator('section.th-card')).toHaveCount(0);await expect(r.getByRole('heading',{name:'银行转账',exact:true})).toHaveCount(0);
   await r.getByRole('button',{name:'添加支付方式',exact:true}).click();const form=r.getByRole('dialog');await form.getByLabel('支付方式',{exact:true}).selectOption('bank_transfer');await form.getByLabel('用于余额和佣金提现').check();await expect(form.getByText('客户填写的收款资料',{exact:true})).toHaveCount(0);await expect(form.getByLabel('付款说明（可选）')).toHaveCount(0);
   await form.getByRole('button',{name:'保存支付方式',exact:true}).click();await expect(form.getByRole('status')).toBeVisible();await form.getByRole('button',{name:'关闭办理面板',exact:true}).click();await r.screenshot({path:info.outputPath(`${role.kind}-manual-payout.png`),fullPage:true});await context.close();
 }
 await p.goto(`/manage/${f.operator}/payment-methods`);await p.getByRole('button',{name:'添加支付方式',exact:true}).click();
 const wechat=p.getByRole('dialog');await wechat.getByLabel('支付方式',{exact:true}).selectOption('wechat');await wechat.getByLabel('用于余额和佣金提现').check();
 await expect(wechat.getByLabel('收款办理方式')).toHaveCount(0);await expect(wechat.getByLabel('提现兑换比例')).toHaveCount(0);await expect(wechat.getByText('线上收款接入',{exact:true})).toHaveCount(0);
 await wechat.getByLabel('手续费规则',{exact:true}).selectOption('percent');await wechat.getByLabel('手续费率 %',{exact:true}).fill('1');
 for(const lang of ['zh','en','ja','ru']){await wechat.getByTestId('language-selector').selectOption(lang);await p.screenshot({path:info.outputPath(`simple-wechat-${lang}.png`),fullPage:true})}
 await wechat.getByTestId('language-selector').selectOption('zh');await wechat.getByRole('button',{name:'保存支付方式',exact:true}).click();await expect(wechat.getByRole('status')).toBeVisible();await wechat.getByRole('button',{name:'关闭办理面板',exact:true}).click();
 await p.goto(`/manage/${f.operator}/commissions`);await expect(p.getByRole('tab',{name:'提现方式',exact:true})).toHaveCount(0);await expect(p.getByRole('tab',{name:'提现申请',exact:true})).toBeVisible();await expect(p.getByText('正在加载当前记录…',{exact:true})).toHaveCount(0);await p.screenshot({path:info.outputPath('commissions-clean.png'),fullPage:true});
 await p.goto(`/manage/${f.operator}/balance-withdrawals`);await expect(p.getByRole('button',{name:'新增提现方式',exact:true})).toHaveCount(0);await expect(p.getByRole('button',{name:'提现方式',exact:true})).toHaveCount(0);
 }catch(error){await p.screenshot({path:info.outputPath('staff-failure.png'),fullPage:true});await c.screenshot({path:info.outputPath('customer-failure.png'),fullPage:true});await info.attach('staff-dom',{body:await p.locator('body').innerText(),contentType:'text/plain'});await info.attach('customer-dom',{body:await c.locator('body').innerText(),contentType:'text/plain'});throw error;}finally{await staff.close();await customer.close()}
});
