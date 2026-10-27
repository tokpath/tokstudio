import {test,expect,type Page} from "@playwright/test";
import {chooseControl} from "./choose-control";
import {readFileSync} from "node:fs";
import {randomUUID} from "node:crypto";
const fixturePath=process.env.TOKENHUB_FRONTEND_ACCEPTANCE_FILE;
test.skip(!fixturePath,"Requires the isolated real application");
async function login(page:Page,email:string,password:string){await page.goto('/login');await expect(page.getByRole('radio')).toHaveCount(0);await page.getByLabel('邮箱',{exact:true}).fill(email);await page.getByLabel('密码',{exact:true}).fill(password);await page.getByRole('button',{name:'登录',exact:true}).click();}
async function optionalSections(page:Page){for(const summary of await page.getByRole('dialog').last().locator('details:not([open]) > summary').all())await summary.click();}
test('new operator, manual owner and first login can be completed without internal IDs or mail',async({browser},info)=>{
 test.setTimeout(120000);const f=JSON.parse(readFileSync(fixturePath!,'utf8'));
 const admin=await browser.newContext(),owner=await browser.newContext();const page=await admin.newPage(),staff=await owner.newPage();page.setDefaultTimeout(15000);staff.setDefaultTimeout(15000);
 try{
 await login(page,f.owner_email,f.owner_password);await expect(page).toHaveURL(/\/manage\//);
 await page.goto(`/manage/${f.operator_id}/operators`);await page.getByRole('button',{name:'建立 OEM / 渠道',exact:true}).click();
 let panel=page.getByRole('dialog').last();await panel.getByRole('combobox',{name:/合作类型/}).selectOption('channel');
 const name='易用性验收渠道 '+randomUUID().slice(0,6),email=`owner-${randomUUID()}@tokenhub.local`;
 await panel.getByLabel('经营方名称',{exact:false}).and(panel.locator('input')).fill(name);await panel.getByLabel('实际经营主体',{exact:false}).and(panel.locator('input')).fill('隔离测试主体');
 await expect(panel.getByLabel(/API.*入口|供货条款编号|退出条款编号/)).toHaveCount(0);
 await panel.getByRole('button',{name:'建立 OEM / 渠道',exact:true}).click();await expect(panel.getByRole('status')).toContainText('经营方已建立');await page.getByRole('button',{name:'关闭办理面板'}).click();
 const row=page.getByRole('row').filter({hasText:name});await row.getByRole('button',{name:'开通首位负责人'}).click();panel=page.getByRole('dialog').last();await optionalSections(page);
 await panel.getByLabel('登录邮箱',{exact:false}).and(panel.locator('input')).fill(email);await panel.getByLabel('新账号初始密码',{exact:false}).and(panel.locator('input')).fill('Initial-password-2026');
 await panel.getByRole('button',{name:'开通首位负责人',exact:true}).click();await expect(panel.getByRole('status')).toContainText('账号已开通');await expect(panel.getByRole('link',{name:'打开登录入口'})).toBeVisible();
 await page.screenshot({path:info.outputPath('manual-owner.png'),fullPage:true});await page.getByRole('button',{name:'关闭办理面板'}).click();await expect(row.getByRole('button',{name:'开通首位负责人'})).toHaveCount(0);
 await login(staff,email,'Initial-password-2026');await expect(staff).toHaveURL(/\/change-password$/);
 await staff.getByLabel('当前密码',{exact:true}).fill('Initial-password-2026');await staff.getByLabel(/^新密码/).fill('Private-password-2026');await staff.getByLabel('确认新密码',{exact:true}).fill('Private-password-2026');await staff.getByRole('button',{name:'保存并进入经营空间'}).click();await expect(staff).toHaveURL(/\/manage\//);
 await expect(staff.locator('body')).toContainText(name);
 }finally{await admin.close().catch(()=>{});await owner.close().catch(()=>{});}
});
test('optional contracts, named procurement choices and generated risk identifiers work through the browser',async({page},info)=>{
 test.setTimeout(120000);page.setDefaultTimeout(15000);const f=JSON.parse(readFileSync(fixturePath!,'utf8'));await login(page,f.owner_email,f.owner_password);await expect(page).toHaveURL(/\/manage\//);
 await page.goto(`/manage/${f.operator_id}/contracts`);await page.getByRole('button',{name:'上传可选合作合同',exact:true}).click();let panel=page.getByRole('dialog').last();
 const filename='验收合作合同.pdf';await panel.locator('input[type=file]').setInputFiles({name:filename,mimeType:'application/pdf',buffer:Buffer.from('%PDF-1.4\n% SYNTHETIC ACCEPTANCE CONTRACT\n%%EOF')});await panel.getByRole('button',{name:'上传可选合作合同',exact:true}).click();await expect(panel.getByRole('status')).toContainText('合同已加密保存');await page.getByRole('button',{name:'关闭办理面板'}).click();await expect(page.getByRole('row').filter({hasText:filename})).toBeVisible();
 await page.goto(`/manage/${f.operator_id}/procurement`);await page.getByRole('tab',{name:'供货报价',exact:true}).click();await page.getByRole('button',{name:'向直属采购方发布供货报价',exact:true}).click();panel=page.getByRole('dialog').last();await optionalSections(page);
 await expect(panel.getByLabel('办理类型',{exact:true})).toHaveCount(0);const buyer=panel.getByRole('combobox',{name:'直属采购方',exact:true});await buyer.focus();await expect(page.getByRole('option',{name:/联调渠道/})).toBeVisible();await chooseControl(buyer,'联调渠道 · 合成渠道主体');
 await panel.getByLabel('收款USD',{exact:false}).and(panel.locator('input')).fill('80');await panel.getByLabel('购买库存ATG',{exact:false}).and(panel.locator('input')).fill('100');await chooseControl(panel.getByRole('combobox',{name:'已上传合作合同（可选）',exact:true}),filename);
 await panel.getByRole('button',{name:'向直属采购方发布供货报价',exact:true}).click();await expect(panel.getByRole('status')).toBeVisible();await expect(panel.locator('[role=alert]')).toHaveCount(0);await page.getByRole('button',{name:'关闭办理面板'}).click();
 await page.goto(`/manage/${f.operator_id}/alerts?view=rules`);await page.getByRole('button',{name:'新增预警规则',exact:true}).click();panel=page.getByRole('dialog').last();await optionalSections(page);
 await expect(panel.getByLabel('规则编号',{exact:false})).toHaveCount(0);await panel.getByLabel('规则名称（留空按指标命名）',{exact:false}).and(panel.locator('input')).fill('运营提前补货提醒');await panel.getByRole('combobox',{name:/真实指标/}).selectOption('pending_usage');await panel.getByLabel('预警阈值',{exact:false}).and(panel.locator('input')).fill('10');await panel.getByLabel('严重阈值',{exact:false}).and(panel.locator('input')).fill('20');await panel.getByRole('combobox',{name:/触发方向/}).selectOption('above');await panel.getByRole('checkbox',{name:/Owner/}).first().check();
 await panel.getByRole('button',{name:'新增预警规则',exact:true}).click();await expect(panel.getByRole('status')).toBeVisible();await expect(panel.locator('[role=alert]')).toHaveCount(0);await page.getByRole('button',{name:'关闭办理面板'}).click();await expect(page.getByRole('row').filter({hasText:'运营提前补货提醒'})).toBeVisible();
 await page.goto(`/manage/${f.operator_id}/coverage`);await page.getByRole('button',{name:/了解预警缓冲/}).first().click();await expect(page.getByRole('dialog').last()).toContainText('超卖额度不计入');await page.screenshot({path:info.outputPath('coverage-help.png'),fullPage:true});
});
