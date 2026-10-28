import {test,expect} from "@playwright/test";
import {readFileSync} from "node:fs";
import {randomUUID} from "node:crypto";
import {translate} from "../lib/i18n";
const ready=process.env.TOKENHUB_INVITATION_ACCEPTANCE_FILE;
test.skip(!ready,"Requires the isolated channel invitation application and PostgreSQL");
test("channel shares its brand invite and queries changes made by another session without changing filters",async({page,browser,baseURL},info)=>{
  const f=JSON.parse(readFileSync(ready!,"utf8"));
  expect(f.invite_code).toMatch(/^[0-9A-Z]{6}$/);
  await page.goto("/login");await page.getByLabel("邮箱",{exact:true}).fill(f.owner_email);await page.getByLabel("密码",{exact:true}).fill(f.owner_password);await page.getByRole("button",{name:"登录",exact:true}).click();await page.waitForURL(/\/manage\//);
  await page.goto(`/manage/${f.operator_id}/overview`);
  await expect(page.getByRole("heading",{name:"渠道邀请",exact:true})).toBeVisible();
  await page.goto(`/manage/${f.operator_id}/customers`);
  await expect(page.getByText(f.invite_code,{exact:true})).toBeVisible();
  await page.context().grantPermissions(["clipboard-read","clipboard-write"]);
  await page.getByRole("button",{name:"复制渠道邀请链接",exact:true}).click();
  const link=await page.evaluate(()=>navigator.clipboard.readText());
  expect(link).toBe(`${baseURL}/register?invite=${f.invite_code}`);
  for(const locale of ["en","ja","ru","zh"] as const){await page.getByTestId("language-selector").selectOption(locale);await expect(page.getByRole("heading",{name:translate(locale,"渠道邀请"),exact:true})).toBeVisible();await expect(page.getByText(f.invite_code,{exact:true})).toBeVisible();}
  await page.screenshot({path:info.outputPath("channel-invitation-desktop.png"),fullPage:true});
  await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:info.outputPath("channel-invitation-mobile.png"),fullPage:true});await page.setViewportSize({width:1365,height:960});
  const anonymous=await browser.newContext({baseURL,locale:"zh-CN"});
  try {const signup=await anonymous.newPage();await signup.goto(link);await expect(signup.getByLabel("邀请码（可选）",{exact:true})).toHaveValue(f.invite_code);await signup.getByLabel("邀请码（可选）",{exact:true}).fill(f.invite_code.toLowerCase());await expect(signup.getByLabel("邀请码（可选）",{exact:true})).toHaveValue(f.invite_code);await expect(signup.getByText("无用户邀请人",{exact:false})).toBeVisible();await expect(signup.getByText("Invite channel",{exact:false})).toBeVisible();} finally {await anonymous.close();}
  await page.goto(`/manage/${f.operator_id}/payments`);
  await expect(page.getByText("暂无记录",{exact:true})).toBeVisible();
  const other=await browser.newContext({baseURL,locale:"zh-CN"});
  try {
    const origin=new URL(baseURL!).origin;
    const logged=await other.request.post("/api/v1/auth/login",{data:{email:f.owner_email,password:f.owner_password},headers:{Origin:origin,"Idempotency-Key":randomUUID()}});expect(logged.status()).toBe(200);
    const me=await(await other.request.get("/api/v1/auth/me")).json();
    const write=async(amount:string)=>{const paid=await other.request.post(`/api/v1/operators/${f.operator_id}/payments`,{data:{gross_usd:amount,currency:"USD",paid_at:new Date().toISOString()},headers:{Origin:origin,"Idempotency-Key":randomUUID(),"X-CSRF-Token":me.csrf_token}});expect(paid.status()).toBe(200);};
    const query=async(count:number)=>{const result=page.waitForResponse(r=>r.request().method()==="GET"&&new URL(r.url()).pathname===`/api/v1/operators/${f.operator_id}/payments`);await page.getByRole("button",{name:"查询",exact:true}).click();expect((await(await result).json()).items).toHaveLength(count);await expect(page.getByRole("button",{name:"查询",exact:true})).toBeEnabled();};
    await write("100");await expect(page.getByText("暂无记录",{exact:true})).toBeVisible();await query(1);await expect(page.getByLabel("100 USD",{exact:true}).first()).toBeVisible();
    await page.getByLabel("筛选状态",{exact:true}).selectOption("pending");await expect(page.getByRole("button",{name:"查询",exact:true})).toBeEnabled();
    await write("200");await query(2);await expect(page.getByLabel("200 USD",{exact:true}).first()).toBeVisible();await expect(page.getByLabel("筛选状态",{exact:true})).toHaveValue("pending");
    await page.screenshot({path:info.outputPath("unchanged-query-refreshed.png"),fullPage:true});
  } finally {await other.close();}
});
