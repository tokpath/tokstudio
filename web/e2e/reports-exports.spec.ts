import {test,expect} from "@playwright/test";
import {readFileSync} from "node:fs";
import {translate} from "../lib/i18n";
const ready=process.env.TOKENHUB_REPORTS_ACCEPTANCE_FILE;
test.skip(!ready,"Requires the isolated reports fixture and PostgreSQL");
test("operator reviews sales then exports the same period as a private CSV",async({page},info)=>{
  const f=JSON.parse(readFileSync(ready!,"utf8"));
  await page.goto("/login");await page.getByLabel("邮箱",{exact:true}).fill(f.owner_email);await page.getByLabel("密码",{exact:true}).fill(f.owner_password);await page.getByRole("button",{name:"登录",exact:true}).click();await page.waitForURL(/\/manage\//);
  await page.goto(`/manage/${f.operator_id}/reports`);
  await expect(page.getByRole("heading",{name:"销售与调用",exact:true})).toBeVisible();
  const sales=page.locator("dl").filter({hasText:"批发销售额"});await expect(sales.getByLabel("1000 USD",{exact:true})).toBeVisible();
  await expect(page.getByText("接管资金明细",{exact:true})).toHaveCount(0);
  await expect(page.locator('input[type="datetime-local"]')).toHaveCount(0);
  await page.getByRole("button",{name:"近7天",exact:true}).click();
  await page.screenshot({path:info.outputPath("reports-desktop.png"),fullPage:true});
  const reportExport=page.getByRole("link",{name:"导出本期数据",exact:true});const target=new URL(await reportExport.getAttribute("href")||"",page.url());
  await reportExport.click();
  await expect(page.getByRole("combobox",{name:"导出内容",exact:true})).toHaveValue("financial");
  await expect(page.getByText("限定主体编号",{exact:false})).toHaveCount(0);
  const submitted=page.waitForResponse(r=>r.request().method()==="POST"&&r.url().endsWith("/exports"));await page.getByRole("button",{name:"生成 CSV",exact:true}).click();
  const result=await submitted;expect(result.status()).toBe(200);const exportResult=await result.json();const body=result.request().postDataJSON();expect(body).toEqual({kind:"financial",from:target.searchParams.get("from"),to:target.searchParams.get("to")});
  const download=page.getByRole("link",{name:"下载 CSV",exact:true}).and(page.locator(`[href="/api/v1/operators/${f.operator_id}/exports/${exportResult.id}/download"]`));await expect(download).toBeVisible({timeout:15_000});
  const event=page.waitForEvent("download");await download.click();const file=await event;const path=info.outputPath("financial.csv");await file.saveAs(path);const csv=readFileSync(path,"utf8");expect(csv).toContain("批发销售额,1000,结算币种");expect(file.suggestedFilename()).toMatch(/\.csv$/);
  await page.screenshot({path:info.outputPath("exports-desktop.png"),fullPage:true});
  await page.setViewportSize({width:390,height:844});await expect(download).toBeVisible();await page.screenshot({path:info.outputPath("exports-mobile.png"),fullPage:true});
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);
  await page.goto(`/manage/${f.operator_id}/reports`);await expect(page.getByRole("heading",{name:"销售与调用",exact:true})).toBeVisible();await page.screenshot({path:info.outputPath("reports-mobile.png"),fullPage:true});
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);
  for (const locale of ["en","ja","ru","zh"] as const) {
    await page.getByTestId("language-selector").selectOption(locale);
    await expect(page.getByRole("heading",{name:translate(locale,"经营报表"),exact:true})).toBeVisible();
    const csv=await page.request.get(`/api/v1/operators/${f.operator_id}/exports/${exportResult.id}/download`);
    expect(csv.status()).toBe(200);expect(await csv.text()).toContain(["分类","指标","数值","单位"].map(label=>translate(locale,label)).join(","));
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);
  }
});
