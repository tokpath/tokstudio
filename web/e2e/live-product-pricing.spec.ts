import { test, expect, type Page, type Locator } from "@playwright/test";
import { readFileSync } from "node:fs";
import { randomUUID } from "node:crypto";

const fixturePath = process.env.TOKENHUB_FRONTEND_ACCEPTANCE_FILE;
test.skip(!fixturePath, "Requires an isolated application and independent model/payment service");

async function submit(page:Page, panel:Locator, label:string, path:string) {
  const waiting=page.waitForResponse(response=>new URL(response.url()).pathname===path && response.request().method()==="POST",{timeout:15_000});
  await panel.getByRole("button",{name:label,exact:true}).click();
  const response=await waiting, data=await response.json();
  expect(response.status(),JSON.stringify(data)).toBe(200);
  await panel.getByRole("button",{name:"关闭办理面板",exact:true}).click();
  return data;
}

test("operators prepare multimodal prices and manage packages without notification paperwork or internal IDs",async({page},info)=>{
  test.setTimeout(180_000);
  const f=JSON.parse(readFileSync(fixturePath!,"utf8")), base=`/api/v1/operators/${f.operator_id}`;
  const errors:string[]=[];page.on("pageerror",error=>errors.push(error.message));
  await page.goto("/login");
  await page.getByLabel("邮箱",{exact:true}).fill(f.owner_email);
  await page.getByLabel("密码",{exact:true}).fill(f.owner_password);
  await page.getByRole("button",{name:"登录",exact:true}).click();
  await expect(page).toHaveURL(/\/manage\//);

  const model=`product-${randomUUID().slice(0,8)}`, name=`通用套餐 ${randomUUID().slice(0,8)}`;
  await page.goto(`/manage/${f.operator_id}/catalog`);
  await page.getByRole("button",{name:"添加公开模型",exact:true}).click();
  let panel=page.getByRole("dialog").last();
  await panel.locator('[data-config-path="model.id"] input').fill(model);
  await panel.getByRole("checkbox",{name:"对话",exact:true}).check();
  await submit(page,panel,"添加公开模型",`${base}/catalog`);

  await page.goto(`/manage/${f.operator_id}/pricing`);
  const card=page.locator("section.th-card").filter({has:page.getByRole("heading",{name:model,exact:true})});
  await expect(card).toBeVisible();
  await card.getByRole("button",{name:"计费模板",exact:true}).click();
  panel=page.getByRole("dialog").last();
  await expect(panel.locator('[data-config-path="evidence"]')).toHaveCount(0);
  await expect(panel.locator('[data-config-path="effective_at"]')).toHaveCount(1);
  await panel.locator('[data-config-path="billing.input"]').getByRole("radio",{name:"按模态组合",exact:true}).check();
  await panel.getByRole("checkbox",{name:"输入计量 · 图片",exact:true}).check();
  await panel.getByRole("combobox",{name:"输入计量 · 图片 · 计量方式",exact:true}).selectOption("megapixels");
  await panel.locator('[data-config-path="billing.output"]').getByRole("radio",{name:"按模态组合",exact:true}).check();
  await submit(page,panel,"设置计费模板",`${base}/pricing`);

  await card.getByRole("button",{name:"设置价格",exact:true}).click();
  panel=page.getByRole("dialog").last();
  await expect(panel.locator('[data-config-path="price.notice_id"],[data-config-path="evidence"]')).toHaveCount(0);
  await expect(panel.locator('[data-config-path="price.effective_at"]')).toHaveCount(1);
  await expect(panel.getByRole("option",{name:"跟随上级供货价",exact:true})).toHaveCount(0);
  const rates=panel.locator('[data-config-path^="price.rule.components."][data-config-path$=".rate"] input');
  await expect(rates).toHaveCount(4);
  for(const input of await rates.all()) await input.fill("0.5");
  const first=await submit(page,panel,"设置模型价格",`${base}/pricing`);
  expect(first.status).toBe("published");
  await expect(card.getByText("客户零售价 · 常规价格",{exact:true})).toBeVisible();
  await card.getByRole("button",{name:"调整价格",exact:true}).click();
  panel=page.getByRole("dialog").last();
  await expect(panel.locator('[data-config-path="price.rule.components.0.rate"] input')).toHaveValue("0.5");
  await panel.locator('[data-config-path="price.rule.components.0.rate"] input').fill("0.6");
  const soon=new Date(Date.now()+60_000);
  await panel.locator('[data-config-path="price.effective_at"] input').fill(new Date(soon.getTime()-soon.getTimezoneOffset()*60000).toISOString().slice(0,16));
  const revised=await submit(page,panel,"设置模型价格",`${base}/pricing`);
  expect(revised.status).toBe("applied");
  await card.screenshot({path:info.outputPath("model-pricing.png")});

  await page.goto(`/manage/${f.operator_id}/plans`);
  await page.getByRole("button",{name:"创建套餐",exact:true}).click();
  panel=page.getByRole("dialog").last();
  await expect(panel.locator('[data-command-field="package_id"],[data-command-field="timezone"],[data-command-field="renewal_enabled"]')).toHaveCount(0);
  for(const [key,value] of Object.entries({name,price_money:"80",paid_atg:"80",gift_atg:"20"})) await panel.locator(`[data-command-field="${key}"] input`).fill(value);
  const created=await submit(page,panel,"创建套餐",`${base}/plans`);
  const plan=page.locator("section.th-card").filter({has:page.getByRole("heading",{name,exact:true})});
  await expect(plan.getByText("无需续费",{exact:true})).toBeVisible();
  await plan.getByRole("button",{name:"编辑套餐",exact:true}).click();
  panel=page.getByRole("dialog").last();
  await panel.locator('[data-command-field="period"] select').selectOption("month");
  await panel.locator('[data-command-field="renewal_enabled"]').getByRole("radio",{name:"是",exact:true}).check();
  const edited=await submit(page,panel,"编辑套餐",`${base}/plans`);
  expect(edited.id).toBe(created.id);
  await expect(plan.getByText("可由客户授权续费",{exact:true})).toBeVisible();
  await plan.getByRole("button",{name:"编辑套餐",exact:true}).click();
  panel=page.getByRole("dialog").last();
  await panel.locator('[data-command-field="new_purchase_enabled"]').getByRole("radio",{name:"否",exact:true}).check();
  await submit(page,panel,"编辑套餐",`${base}/plans`);
  await expect(plan.getByText("已停止新购",{exact:true})).toBeVisible();
  await expect(plan.getByText("可由客户授权续费",{exact:true})).toBeVisible();
  await plan.screenshot({path:info.outputPath("package-management.png")});
  await page.setViewportSize({width:390,height:844});
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);
  expect(errors).toEqual([]);
  await info.attach("business-results",{body:JSON.stringify({model,first,revised,created,edited}),contentType:"application/json"});
});
